package dashboard

import "net/http"

const testPageHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>PinchTab Test</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0a0a0f;color:#e0e0e0;font-family:system-ui,sans-serif;overflow:hidden}
canvas{position:fixed;top:0;left:0;width:100%;height:100%}
.ui{position:relative;z-index:1;display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100vh;gap:16px;pointer-events:none}
.logo{font-size:24px;font-weight:700;letter-spacing:3px;color:#fff}
.logo span{color:#00d4ff}
.btn{pointer-events:auto;background:#00d4ff22;border:1px solid #00d4ff44;color:#00d4ff;padding:10px 28px;border-radius:8px;font-size:13px;font-weight:600;cursor:pointer}
.btn:hover{background:#00d4ff33}
.btn:active{transform:scale(.97)}
.n{font-size:12px;color:#555;font-family:monospace}
.h{font-size:10px;color:#333;position:fixed;bottom:12px;left:50%;transform:translateX(-50%)}
</style></head><body>
<canvas id="c"></canvas>
<div class="ui">
<div class="logo">PINCH<span>TAB</span></div>
<button class="btn" id="b">Click me</button>
<div class="n" id="n">0 clicks</div>
</div>
<div class="h">Click anywhere to draw · Scroll to resize</div>
<script>
var c=document.getElementById('c'),x=c.getContext('2d'),k=0,r=12;
function sz(){c.width=innerWidth;c.height=innerHeight}sz();
onresize=sz;
onclick=function(e){x.beginPath();x.arc(e.clientX,e.clientY,r,0,6.28);x.fillStyle='hsl('+(k*30%360)+',80%,55%)';x.fill();k++;document.getElementById('n').textContent=k+' click'+(k!==1?'s':'')};
onwheel=function(e){r=Math.max(3,Math.min(50,r+(e.deltaY>0?2:-2)))};
document.getElementById('b').onclick=function(e){e.stopPropagation();var b=e.target;b.textContent='Clicked!';b.style.color='#0f8';setTimeout(function(){b.textContent='Click me';b.style.color=''},1500)};
</script></body></html>`

func handleTestPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(testPageHTML))
}
