import http from 'node:http';
import { appendFile, mkdir } from 'node:fs/promises';
import { dirname } from 'node:path';

const port = Number(process.env.PORT || 8080);
const logFile = process.env.LOG_FILE;

async function logRequest(req) {
  if (!logFile) return;
  const entry = {
    ts: new Date().toISOString(),
    method: req.method,
    path: req.url,
    userAgent: req.headers['user-agent'] || '',
    remoteAddress: req.socket.remoteAddress || '',
  };
  await mkdir(dirname(logFile), { recursive: true });
  await appendFile(logFile, JSON.stringify(entry) + '\n');
}

function send(res, status, body, contentType = 'text/html; charset=utf-8', extraHeaders = {}) {
  res.writeHead(status, { 'content-type': contentType, ...extraHeaders });
  res.end(body);
}

function page(title, body, extraHead = '') {
  return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>${title}</title>
    ${extraHead}
  </head>
  <body>
    ${body}
  </body>
</html>`;
}

const server = http.createServer(async (req, res) => {
  await logRequest(req);
  const url = new URL(req.url, `http://127.0.0.1:${port}`);

  if (url.pathname === '/health') {
    return send(res, 200, 'ok', 'text/plain; charset=utf-8');
  }

  if (url.pathname === '/alpha') {
    return send(
      res,
      200,
      page('Alpha Verification', '<main><h1>ALPHA-17</h1><p>This is the first verification page.</p></main>'),
    );
  }

  if (url.pathname === '/journey/start') {
    return send(
      res,
      200,
      page(
        'Journey Start',
        `<main>
          <h1>Journey Start</h1>
          <p>Press the button to continue.</p>
          <button id="begin">Begin journey</button>
        </main>
        <script>
          document.getElementById('begin').addEventListener('click', () => {
            document.cookie = 'journey=1; Path=/; SameSite=Lax';
            window.location.href = '/journey/final';
          });
        </script>`,
      ),
    );
  }

  if (url.pathname === '/journey/final') {
    const hasCookie = (req.headers.cookie || '').split(/;\s*/).includes('journey=1');
    if (!hasCookie) {
      return send(
        res,
        200,
        page('Journey Incomplete', '<main><h1>MISSING-STATE</h1><p>Start from the journey page first.</p></main>'),
      );
    }
    return send(
      res,
      200,
      page('Journey Complete', '<main><h1>ORBIT-42</h1><p>Browser state carried across correctly.</p></main>'),
    );
  }

  if (url.pathname === '/chain/one') {
    return send(
      res,
      200,
      page('Chain One', '<main><h1>Chain One</h1><a href="/chain/two">Go to step two</a></main>'),
    );
  }

  if (url.pathname === '/chain/two') {
    return send(
      res,
      200,
      page('Chain Two', '<main><h1>Chain Two</h1><a href="/chain/final">Open the final page</a></main>'),
    );
  }

  if (url.pathname === '/chain/final') {
    return send(
      res,
      200,
      page('Chain Final', '<main><h1>BLUE-SUN-9</h1><p>You made it through the chain.</p></main>'),
    );
  }

  return send(res, 404, page('Not Found', `<main><h1>404</h1><p>No route for ${url.pathname}</p></main>`));
});

server.listen(port, '0.0.0.0', () => {
  console.log(`fixture server listening on ${port}`);
});
