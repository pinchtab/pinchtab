#!/usr/bin/env python3
"""Parse arXiv search results from pinchtab text output."""
import sys, re, json

def parse_page(input_file, output_file):
    with open(input_file) as f:
        data = json.load(f)
    text = data.get("text", "")

    papers = re.split(r'(?=arXiv:\d{4}\.\d{4,5})', text)

    count = 0
    with open(output_file, 'a') as f:
        for chunk in papers:
            id_match = re.search(r'(arXiv:\d{4}\.\d{4,5})', chunk)
            if not id_match:
                continue

            arxiv_id = id_match.group(1)
            cats = re.findall(r'\b(cs\.\w{2}|eess\.\w{2}|stat\.\w{2}|math\.\w{2}|quant-ph)\b', chunk)
            primary_cat = cats[0] if cats else ""
            all_cats = list(dict.fromkeys(cats))

            title = ""
            # Title is between the last category line and "Authors:"
            # Categories appear as "cs.AI cs.CL cs.LG" then title on next non-blank line
            block = re.search(r'(?:cs\.\w{2}|eess\.\w{2}|stat\.\w{2}|quant-ph)\s*\n(.*?)Authors:', chunk, re.DOTALL)
            if block:
                lines = [l.strip() for l in block.group(1).strip().split('\n') if l.strip()]
                # Filter out lines that are just category codes
                title_lines = [l for l in lines if not re.match(r'^(cs|eess|stat|math|quant)\.\w{2}$', l)]
                title = title_lines[0] if title_lines else ""

            authors = ""
            authors_match = re.search(r'Authors:\s*\n(.*?)(?:\n\s*Abstract:|\n\s*Submitted)', chunk, re.DOTALL)
            if authors_match:
                raw = authors_match.group(1)
                names = re.findall(r'([A-Z][a-z]+(?: [A-Z]\.?)*(?: (?:de |van |Von |Le |El )?[A-Za-z-]+)+)', raw)
                authors = ", ".join(names)

            abstract = ""
            abs_match = re.search(r'Abstract:\s*\n\s*(.*?)(?:△ Less|\n\s*Submitted)', chunk, re.DOTALL)
            if abs_match:
                abstract = re.sub(r'\s+', ' ', abs_match.group(1)).strip()
                abstract = re.sub(r'[▽△]\s*(More|Less)', '', abstract).strip()

            date_match = re.search(r'Submitted\s+(\d+ \w+, \d{4})', chunk)
            date = date_match.group(1) if date_match else ""

            comments = ""
            comm_match = re.search(r'Comments:\s*\n\s*(.+?)(?:\n\s*\n|\n\s*Journal|\n\s*ACM|$)', chunk, re.DOTALL)
            if comm_match:
                comments = comm_match.group(1).strip()

            paper = {
                "id": arxiv_id,
                "url": f"https://arxiv.org/abs/{arxiv_id.replace('arXiv:', '')}",
                "title": title,
                "authors": authors,
                "categories": all_cats,
                "primary_category": primary_cat,
                "abstract": abstract[:500] + ("..." if len(abstract) > 500 else ""),
                "date": date,
                "comments": comments,
            }

            f.write(json.dumps(paper) + '\n')
            count += 1

    return count


def generate_summary(jsonl_file, output_dir):
    from collections import Counter

    papers = []
    with open(jsonl_file) as f:
        for line in f:
            if line.strip():
                papers.append(json.loads(line))

    cats = Counter()
    for p in papers:
        for c in p.get("categories", []):
            cats[c] += 1

    with open(f"{output_dir}/summary.md", 'w') as f:
        f.write(f"# arXiv: AI Agent Memory\n\n")
        f.write(f"**Total papers:** {len(papers)}\n\n")
        f.write(f"## Categories\n\n")
        for cat, count in cats.most_common(15):
            f.write(f"- `{cat}`: {count}\n")
        f.write(f"\n## Papers\n\n")
        for i, p in enumerate(papers, 1):
            f.write(f"### {i}. {p['title']}\n\n")
            f.write(f"- **ID:** [{p['id']}]({p['url']})\n")
            f.write(f"- **Authors:** {p['authors']}\n")
            f.write(f"- **Date:** {p['date']}\n")
            if p.get('comments'):
                f.write(f"- **Comments:** {p['comments']}\n")
            f.write(f"- **Abstract:** {p['abstract']}\n\n")

    with open(f"{output_dir}/papers.json", 'w') as f:
        json.dump(papers, f, indent=2)

    print(f"📊 {output_dir}/summary.md")
    print(f"📋 {output_dir}/papers.json")


if __name__ == "__main__":
    cmd = sys.argv[1]
    if cmd == "parse":
        count = parse_page(sys.argv[2], sys.argv[3])
        print(count)
    elif cmd == "summary":
        generate_summary(sys.argv[2], sys.argv[3])
