package opt

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// expectPatterns maps step ID → regex pattern for answer verification.
// Ported from verify-answers.sh — patterns are case-insensitive.
var expectPatterns = map[string]string{
	// Group 0: Setup & diagnosis
	"0.1": `status.*ok|\bok\b|"ok"`,
	"0.2": `401|missing_token|unauth|bad_token|rejected`,
	"0.3": `200|authed.*200|status.*ok|\bok\b|"ok"`,
	"0.4": `running`,
	"0.5": `tab.*list|tabs.*returned|active tab|listed.*tab|\d+ tabs? (found|open)|tabs:|tab.*id|"tabs"\s*:|\bno (open )?tabs\b|empty`,
	"0.6": `cleaned|closed|no stale|reused.*tab|no cleanup|single active|is empty|already empty|clean.*state|cannot close last|one tab`,
	"0.7": `VERIFY_HOME_LOADED_12345|navigated to fixtures|fixtures.*home|PinchTab Benchmark|fixtures/`,
	"0.8": `[0-9A-F]{32}|tab.*id.*captured`,
	// Group 1: Reading & Extracting
	"1.1": `COUNT_LANGUAGES_12|Programming Languages 12|12 articles`,
	"1.2": `wiki-go|Go \(programming language\)|409 success|clicked`,
	"1.3": `Robert Griesemer.*2009|2009.*Griesemer`,
	"1.4": `FEATURE_COUNT_6|6 key features|6 features`,
	"1.5": `Artificial Intelligence.*Climate Action.*Mars|Artificial Intelligence, Climate Action`,
	"1.6": `24,582.*1,284,930|24,582|Revenue \$1,284,930`,
	// Group 2: Search
	"2.1": `\bA\b|wiki-go|VERIFY_WIKI_GO_LANG_88888`,
	"2.2": `No results|no results|xyznonexistent`,
	"2.3": `Artificial Intelligence|ARTIFICIAL_INTELLIGENCE`,
	// Group 3: Form
	"3.1": `submitted|VERIFY_FORM_SUBMITTED_SUCCESS|FORM_SUBMITTED|SUBMISSION_DATA`,
	"3.2": `RESET_BUTTON_VISIBLE`,
	// Group 4: SPA
	"4.1": `TASK_STATS_TOTAL_3|Total.*3.*Active.*2.*Done.*1|3.*2.*1`,
	"4.2": `TASK_ADDED|AUTOMATE|DEPLOYMENT|high`,
	"4.3": `deleted.*task|task.*deleted|TASK_STATS_TOTAL_3|All Tasks.*3|3.*tasks|4 to 3|tests deleted|count.*3`,
	// Group 5: Login
	"5.1": `INVALID_CREDENTIALS_ERROR|Invalid`,
	"5.2": `VERIFY_LOGIN_SUCCESS_DASHBOARD|SESSION_TOKEN_ACTIVE_TRUE|login success|Dashboard`,
	// Group 6: E-commerce
	"6.1": `149.99.*299.99|Wireless.*Smart Watch|Portable Charger`,
	"6.2": `449.98|CART_ITEM_WIRELESS`,
	"6.3": `VERIFY_CHECKOUT_SUCCESS_ORDER|checkout`,
	// Group 7: Content + Interaction
	"7.1": `COMMENT_POSTED_RATING_5`,
	"7.2": `Developer Tools.*15|15.*Developer|wiki-go|VERIFY_WIKI_GO`,
	// Group 8: Error handling
	"8.1": `404|not found`,
	"8.2": `element.*not found|no element found|no element|clear error|not found|error.*500|ref.*not found`,
	// Group 9: Export
	"9.1": `[Ss]creenshot|\.png.*[0-9]{4,} bytes`,
	"9.2": `[Pp][Dd][Ff]|\.pdf.*[0-9]{4,} bytes`,
	// Group 10: Modals
	"10.1": `Dashboard Settings`,
	"10.2": `THEME_DARK_APPLIED|[Dd]ark.*theme.*applied|theme.*[Dd]ark`,
	// Group 11: Persistence
	"11.1": `\bA\b|TASK_PERSISTENT_TEST_FOUND|[Pp]ersistent.*[Tt]ask.*[Tt]est.*found|found.*[Pp]ersistent`,
	"11.2": `\bA\b|SESSION_RENEWED|VERIFY_LOGIN_SUCCESS_DASHBOARD|[Ww]elcome.*back|[Dd]ashboard.*shown|logged.*in.*success|login.*success`,
	// Group 12: Multi-page nav
	"12.1": `home|VERIFY_HOME_LOADED_12345|PinchTab Benchmark|fixtures/$|fixtures/\s`,
	"12.2": `COUNT_LANGUAGES_12|12.*Programming|Artificial Intelligence|Total Articles|12\+8\+15|COMPARISON_DATA_FOUND`,
	// Group 13: Form validation
	"13.1": `blocked|display:none|valueMissing|not submitted|[Vv]alidation.*[Ee]rror|required|missing.*field|form.*invalid`,
	"13.2": `VERIFY_FORM_SUBMITTED_SUCCESS|OPTIONAL_FIELD_SKIPPED_SUCCESS|submitted`,
	// Group 14: Dynamic content
	"14.1": `ADDITIONAL_PRODUCTS_LOADED|USB-C|Screen Protector|new.*products|products.*appeared|more.*products|loaded.*products`,
	"14.2": `CART_UPDATED_WITH_LAZY_PRODUCT|USB-C`,
	// Group 15: Data aggregation
	"15.1": `1,284,930.*384,930|PROFIT_MARGIN_CALCULATED|Revenue.*Profit`,
	"15.2": `FEATURE_COUNT_6.*7.*5|Go=6.*Python=7.*Rust=5|6, 7, 5|COMPARISON_TABLE_BUILT|Go.*6.*Python.*7.*Rust.*5`,
	"15.3": `VERIFY_ARTICLE_PAGE_41414|<article|<h[12]|article.*html|html.*article|extracted.*html`,
	"15.4": `COMPUTED_DISPLAY_FLEX`,
	// Group 16: Hover
	"16.1": `HOVER_REVEALED_USER_1`,
	"16.2": `HOVER_REVEALED_USER_2`,
	// Group 17: Scroll
	"17.1": `SCROLL_MIDDLE_MARKER`,
	"17.2": `SCROLL_REACHED_FOOTER`,
	// Group 18: Download
	"18.1": `DOWNLOAD_FILE_CONTENT_VERIFIED|143 bytes`,
	// Group 19: iFrame
	"19.1": `IFRAME_INNER_CONTENT_LOADED`,
	"19.2": `IFRAME_INPUT_RECEIVED_HELLO_WORLD`,
	// Group 20: Dialogs
	"20.1": `DIALOG_ALERT_DISMISSED|[Aa]lert.*dismissed|dismissed.*alert|alert.*accepted|accepted.*alert`,
	"20.2": `DIALOG_CONFIRM_CANCELLED|[Cc]onfirm.*cancell|cancell.*confirm|confirm.*denied|denied.*confirm`,
	// Group 21: Async
	"21.1": `ASYNC_PAYLOAD_READY_42`,
	"21.2": `ASYNC_USER_NAME_ADA`,
	// Group 22: Drag
	"22.1": `DROP_ZONE_A_OK`,
	"22.2": `DROP_SEQUENCE=DROP_ZONE_A_OK,DROP_ZONE_B_OK,DROP_ZONE_C_OK`,
	// Group 23: Loading
	"23.1": `VERIFY_LOADING_COMPLETE_88888`,
	// Group 24: Keyboard
	"24.1": `KEYBOARD_ESCAPE_PRESSED`,
	"24.2": `KEYBOARD_KEY_A_PRESSED|KEYBOARD_ENTER_PRESSED|ESC.*A.*ENTER`,
	// Group 25: Tabs
	"25.1": `TAB_SETTINGS_CONTENT`,
	"25.2": `TAB_BILLING_CONTENT`,
	// Group 26: Accordion
	"26.1": `ACCORDION_SECTION_A_OPEN`,
	"26.2": `ACCORDION_SECTION_B_OPEN.*aria-expanded=false|Section A aria-expanded=false|ACCORDION_SECTION_B_OPEN.*collapsed|Section B.*open.*Section A.*closed|Section B.*open.*Section A.*collapsed`,
	// Group 27: Editor
	"27.1": `EDITOR_CHARS=15|Hello rich text`,
	"27.2": `EDITOR_COMMITTED=Hello rich text`,
	// Group 28: Range
	"28.1": `RANGE_VALUE_90.*BUCKET_HIGH|RANGE_VALUE_90`,
	"28.2": `RANGE_VALUE_10.*BUCKET_LOW|RANGE_VALUE_10`,
	// Group 29: Pagination
	"29.1": `PAGE_2_FIRST_ITEM|PAGE_2_OF_3`,
	"29.2": `PAGE_3_FIRST_ITEM|disabled=true`,
	// Group 30: Dropdown
	"30.1": `DROPDOWN_SELECTED=BETA`,
	"30.2": `DROPDOWN_SELECTED=GAMMA`,
	// Group 31-34: Iframe variants
	"31.1": `DEEP_CLICKED=YES_LEVEL_3`,
	"32.1": `IFRAME_INPUT_RECEIVED_LATE_WORLD|Late World.*submitted|filled.*Late World|Late World.*saved`,
	"33.1": `INLINE_RECEIVED_SRCDOC|srcdoc.*submitted|filled.*srcdoc|srcdoc.*saved`,
	"34.1": `SANDBOX_CLICKED=YES`,
	// Group 35: Text-heavy
	"35.1": `ARTICLE_PUBLISHED_2026_04_15.*ARTICLE_WORD_COUNT_MARKER_323|ARTICLE_WORD_COUNT_MARKER_323`,
	"35.2": `FOOTER_COPYRIGHT_MARKER`,
	// Group 36: SERP
	"36.1": `RESULT_3_TITLE.*RESULT_3_SNIPPET_MARKER|RESULT_3`,
	"36.2": `RESULT_1.*RESULT_6|SERP_RESULT_COUNT_6`,
	// Group 37: Q&A
	"37.1": `a-2|ACCEPTED_ANSWER.*A2|answer.*2|id.*a.?2`,
	"37.2": `ANSWER_2_BODY_MARKER|ACCEPTED_ANSWER_ID_A2`,
	// Group 38: Pricing
	"38.1": `PLAN_PRO_PRICE_29`,
	"38.2": `PLAN_FREE_PRICE_0.*PLAN_PRO_PRICE_29.*PLAN_ENTERPRISE_PRICE_CUSTOM|PLAN_PRO_PRICE_29`,
	// Group 39: File Upload
	"39.1": `UPLOAD_FILE_RECEIVED.*UPLOAD_COUNT=1|uploaded`,
	// Group 40: Double-click Inline Edit
	"40.1": `CELL_EDITING_MODE_ACTIVE|edit.*mode|editing.*mode|editing.*active|input.*visible`,
	"40.2": `CELL_VALUE_UPDATED=Alicia|updated.*Alicia|Alicia.*saved|Alicia.*committed|^Alicia$`,
	// Group 41: Autocomplete
	"41.1": `SUGGESTIONS_VISIBLE_COUNT_2`,
	"41.2": `AUTOCOMPLETE_SELECTED=Python`,
	// Group 42: Toast Notifications
	"42.1": `TOAST_MESSAGE_SAVED_SUCCESSFULLY|[Ss]aved [Ss]uccessfully|save.*success`,
	"42.2": `TOAST_DISMISSED|toast.*dismiss|dismiss.*toast|toast.*disappear|no.*toast`,
	"42.3": `TOAST_MESSAGE_ERROR_FAILED|[Ee]rror.*[Ff]ailed|error.*toast`,
	// Group 43: Staff Directory
	"43.1": `TOTAL_STAFF_13|13.*staff|staff.*13|VERIFY_DIRECTORY_PAGE_90909`,
	"43.2": `DEPT_EXPANDED=ENG.*ada@company\.test|ada@company\.test.*DEPT_EXPANDED|ada@company\.test.*[Ee]ng|[Ee]ng.*ada@company\.test`,
	// Group 44: Multi-step Wizard
	"44.1": `WIZARD_STEP=2|[Ss]tep 2|[Pp]references`,
	"44.2": `WIZARD_SUMMARY_NAME.*WIZARD_SUMMARY_PLAN=PRO|WIZARD_STEP=3.*WIZARD_SUMMARY_NAME|[Ss]ummary.*PRO|PRO.*plan.*confirm`,
	// Group 45: Element Attributes & State Inspection
	"45.1": `ATTR_BUTTON_COUNT_5`,
	"45.2": `ATTR_TESTID=delete-action`,
	"45.3": `ATTR_DELETE_DISABLED`,
	"45.4": `ATTR_CHECKED_COUNT_3`,
	"45.5": `ATTR_NAME_VALUE_ALICE`,
	"45.6": `ATTR_HIDDEN_MSG_NOT_VISIBLE`,
	"45.7": `ATTR_LINK_COUNT_7`,
	// Group 46: Tab Lifecycle
	"46.1": `TAB_TOTAL_COUNT_3`,
	"46.2": `TAB_REMAINING_COUNT_2`,
}

var groupSizes = map[int]int{
	0: 8, 1: 6, 2: 3, 3: 2, 4: 3, 5: 2, 6: 3, 7: 2, 8: 2, 9: 2, 10: 2, 11: 2, 12: 2,
	13: 2, 14: 2, 15: 4, 16: 2, 17: 2, 18: 1, 19: 2, 20: 2, 21: 2, 22: 2, 23: 1, 24: 2, 25: 2,
	26: 2, 27: 2, 28: 2, 29: 2, 30: 2, 31: 1, 32: 1, 33: 1, 34: 1, 35: 2, 36: 2, 37: 2, 38: 2,
	39: 1, 40: 2, 41: 2, 42: 3, 43: 2, 44: 2, 45: 7, 46: 2,
}

func totalBaselineSteps() int {
	n := 0
	for _, c := range groupSizes {
		n += c
	}
	return n
}

func maxGroup() int {
	m := 0
	for g := range groupSizes {
		if g > m {
			m = g
		}
	}
	return m
}

type VerifyAnswersArgs struct {
	ReportFiles []string
}

func ParseVerifyAnswersArgs(argv []string) (VerifyAnswersArgs, error) {
	var args VerifyAnswersArgs
	args.ReportFiles = argv

	if len(args.ReportFiles) == 0 {
		return args, fmt.Errorf("usage: verify-answers <report1.json> [report2.json ...]")
	}

	return args, nil
}

type verifyResult struct {
	ID      string
	Status  string // "pass", "fail", "skip"
	Answer  string
	Pattern string
}

func RunVerifyAnswers(argv []string, stdout, stderr io.Writer) int {
	args, err := ParseVerifyAnswersArgs(argv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "verify-answers: %v\n", err)
		return 1
	}

	// Expand globs
	var files []string
	for _, pattern := range args.ReportFiles {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			if _, statErr := os.Stat(pattern); statErr == nil {
				files = append(files, pattern)
			} else {
				_, _ = fmt.Fprintf(stderr, "verify-answers: no files matched %q\n", pattern)
				return 1
			}
		} else {
			files = append(files, matches...)
		}
	}

	type stepData struct {
		ID     string `json:"id"`
		Answer string `json:"answer"`
	}
	seen := make(map[string]stepData)

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "verify-answers: failed to read %s: %v\n", f, err)
			return 1
		}
		var report struct {
			Steps []stepData `json:"steps"`
		}
		if err := json.Unmarshal(data, &report); err != nil {
			_, _ = fmt.Fprintf(stderr, "verify-answers: failed to parse %s: %v\n", f, err)
			return 1
		}
		for _, s := range report.Steps {
			seen[s.ID] = s
		}
	}

	// Verify each step against expected pattern
	var results []verifyResult
	for id, s := range seen {
		pattern, hasPattern := expectPatterns[id]
		if !hasPattern {
			results = append(results, verifyResult{
				ID:     id,
				Status: "skip",
				Answer: s.Answer,
			})
			continue
		}

		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "verify-answers: bad pattern for %s: %v\n", id, err)
			results = append(results, verifyResult{
				ID:      id,
				Status:  "fail",
				Answer:  s.Answer,
				Pattern: pattern,
			})
			continue
		}

		if re.MatchString(s.Answer) {
			results = append(results, verifyResult{
				ID:      id,
				Status:  "pass",
				Answer:  s.Answer,
				Pattern: pattern,
			})
		} else {
			results = append(results, verifyResult{
				ID:      id,
				Status:  "fail",
				Answer:  s.Answer,
				Pattern: pattern,
			})
		}
	}

	// Sort results by group.step
	sort.Slice(results, func(i, j int) bool {
		gi, si := parseStepID(results[i].ID)
		gj, sj := parseStepID(results[j].ID)
		if gi != gj {
			return gi < gj
		}
		return si < sj
	})

	// Count totals
	var pass, fail, skip int
	var failures []verifyResult
	for _, r := range results {
		switch r.Status {
		case "pass":
			pass++
		case "fail":
			fail++
			failures = append(failures, r)
		case "skip":
			skip++
		}
	}

	total := pass + fail + skip
	_, _ = fmt.Fprintf(stdout, "Verified: %d/%d steps\n", total, totalBaselineSteps())
	_, _ = fmt.Fprintf(stdout, "  pass: %d\n", pass)
	_, _ = fmt.Fprintf(stdout, "  fail: %d\n", fail)
	_, _ = fmt.Fprintf(stdout, "  skip: %d (no pattern)\n", skip)

	// Find missing steps
	var missing []string
	for g := 0; g <= maxGroup(); g++ {
		count := groupSizes[g]
		for s := 1; s <= count; s++ {
			id := fmt.Sprintf("%d.%d", g, s)
			if _, ok := seen[id]; !ok {
				missing = append(missing, id)
			}
		}
	}

	if len(missing) > 0 {
		_, _ = fmt.Fprintf(stdout, "  missing: %s\n", strings.Join(missing, ", "))
	}

	if len(failures) > 0 {
		_, _ = fmt.Fprintf(stdout, "\nFailed steps:\n")
		for _, f := range failures {
			answer := f.Answer
			if len(answer) > 100 {
				answer = answer[:97] + "..."
			}
			_, _ = fmt.Fprintf(stdout, "  %s: answer=[%s] expected=[%s]\n", f.ID, answer, f.Pattern)
		}
	}

	if fail > 0 {
		return 1
	}
	return 0
}

func parseStepID(id string) (int, int) {
	var g, s int
	_, _ = fmt.Sscanf(id, "%d.%d", &g, &s)
	return g, s
}
