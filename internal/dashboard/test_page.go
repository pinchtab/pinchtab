package dashboard

import "net/http"

const testPageHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Test Page</title>
<style>*{margin:0;padding:0;box-sizing:border-box}body{background:#fff;font-family:system-ui,sans-serif}canvas{position:fixed;top:0;left:0;width:100%;height:100%}.t{position:fixed;top:50%;left:50%;transform:translate(-50%,-50%);font-size:32px;font-weight:700;color:#ccc;letter-spacing:4px;pointer-events:none;z-index:1}</style></head><body>
<canvas id="c"></canvas>
<div class="t">TEST PAGE</div>
<script>
var c=document.getElementById('c'),x=c.getContext('2d');
function sz(){c.width=innerWidth;c.height=innerHeight}sz();
onresize=sz;
onclick=function(e){x.beginPath();x.arc(e.clientX,e.clientY,8,0,6.28);x.fillStyle='red';x.fill()};
</script></body></html>`

func handleTestPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(testPageHTML))
}
