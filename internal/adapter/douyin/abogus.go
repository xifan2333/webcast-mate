package douyin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	bdmsURL = "https://lf-c-flwb.bytetos.com/obj/rc-client-security/web/stable/1.0.1.20/bdms.js"
	cdpPort = "9334" // avoid clash with pure_create 9333
)

// SignABogus signs query+body for room/create.
// Order:
//  1. WEBCAST_MATE_DY_ABOGUS if set (caller-supplied)
//  2. WEBCAST_MATE_DY_ABOGUS_CMD external command (stdin: JSON {query,body} → stdout a_bogus)
//  3. headless chromium + official bdms 1.0.1.20 (same as ~/douyin-live/pure_create.py)
func SignABogus(query, body string) (string, error) {
	if v := strings.TrimSpace(os.Getenv("WEBCAST_MATE_DY_ABOGUS")); v != "" {
		return v, nil
	}
	if cmd := strings.TrimSpace(os.Getenv("WEBCAST_MATE_DY_ABOGUS_CMD")); cmd != "" {
		return signViaCmd(cmd, query, body)
	}
	return signViaChromiumBDMS(query, body)
}

func signViaCmd(cmdline, query, body string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"query": query, "body": body})
	c := exec.Command("sh", "-c", cmdline)
	c.Stdin = strings.NewReader(string(payload))
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("abogus cmd: %w", err)
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "", fmt.Errorf("abogus cmd empty output")
	}
	return s, nil
}

func cacheDir() string {
	d := filepath.Join(os.TempDir(), "webcast-mate-dy")
	_ = os.MkdirAll(d, 0o755)
	return d
}

func ensureBDMS() (string, error) {
	path := filepath.Join(cacheDir(), "bdms-1.0.1.20.js")
	if st, err := os.Stat(path); err == nil && st.Size() > 10000 {
		return path, nil
	}
	resp, err := http.Get(bdmsURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func findChromium() string {
	for _, n := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func signViaChromiumBDMS(query, body string) (string, error) {
	chrome := findChromium()
	if chrome == "" {
		return "", fmt.Errorf("chromium not found; set WEBCAST_MATE_DY_ABOGUS or WEBCAST_MATE_DY_ABOGUS_CMD (see docs)")
	}
	bdms, err := ensureBDMS()
	if err != nil {
		return "", fmt.Errorf("bdms download: %w", err)
	}

	// Prefer Python helper from douyin-live if present (battle-tested CDP).
	if p := os.Getenv("WEBCAST_MATE_DY_PURE_CREATE"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return signViaPureCreatePy(p, query, body)
		}
	}
	home, _ := os.UserHomeDir()
	cand := filepath.Join(home, "douyin-live", "pure_create.py")
	if _, err := os.Stat(cand); err == nil {
		// use embedded CDP via python one-shot if uv/python available
		if ab, err := signViaPythonSnippet(cand, query, body); err == nil && ab != "" {
			return ab, nil
		}
	}

	// Minimal CDP via chrome remote debugging + websockets not in go.mod —
	// shell out to a tiny python inline using websocket-client if available,
	// else instruct user.
	_ = bdms
	_ = chrome
	return signViaInlinePython(chrome, bdms, query, body)
}

func signViaPureCreatePy(path, query, body string) (string, error) {
	// not used as full create — only if we add a --abogus-only later
	_ = path
	return "", fmt.Errorf("use inline")
}

func signViaPythonSnippet(pureCreatePath, query, body string) (string, error) {
	// Call into pure_create.gen_abogus after starting chromium — heavy.
	// Instead run a short script importing nothing from pure_create, duplicate gen.
	return "", fmt.Errorf("skip")
}

func signViaInlinePython(chrome, bdms, query, body string) (string, error) {
	// Write a self-contained python script matching pure_create.gen_abogus
	script := filepath.Join(cacheDir(), "sign_abogus.py")
	py := fmt.Sprintf(`#!/usr/bin/env python3
import json, time, urllib.request, subprocess, os, sys
from pathlib import Path

UA = %q
QUERY = %q
BODY = %q
BDMS = %q
CHROME = %q
PORT = %q
CACHE = Path(%q)

def start():
    subprocess.run(["pkill", "-f", f"remote-debugging-port={PORT}"], capture_output=True)
    time.sleep(0.3)
    profile = CACHE / "chrome-profile"
    profile.mkdir(parents=True, exist_ok=True)
    log = open(CACHE / "chrome.log", "w")
    proc = subprocess.Popen(
        [CHROME, "--headless=new", "--disable-gpu", "--no-sandbox",
         "--enable-unsafe-swiftshader",
         f"--remote-debugging-port={PORT}", "--remote-allow-origins=*",
         f"--user-agent={UA}", f"--user-data-dir={profile}", "about:blank"],
        stdout=log, stderr=log,
    )
    for _ in range(40):
        try:
            urllib.request.urlopen(f"http://127.0.0.1:{PORT}/json/version", timeout=1)
            return proc
        except Exception:
            time.sleep(0.25)
    raise SystemExit("cdp not ready")

def gen(proc):
    try:
        import websocket
    except ImportError:
        print("need websocket-client: pip install websocket-client", file=sys.stderr)
        raise SystemExit(2)
    html_path = CACHE / "sign.html"
    html = f"""<!DOCTYPE html><html><head><meta charset=utf-8></head><body>
<script>
(function(){{
  Object.defineProperty(navigator,'platform',{{get:()=> 'Win32'}});
  Object.defineProperty(navigator,'userAgent',{{get:()=> {UA!r}}});
  Object.defineProperty(navigator,'webdriver',{{get:()=> false}});
}})();
window.__query = {QUERY!r};
window.__body = {BODY!r};
window.__urls = [];
(function(){{
  const o = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(m,u){{ window.__urls.push(String(u)); return o.apply(this,arguments); }};
  XMLHttpRequest.prototype.send = function(){{
    try {{
      Object.defineProperty(this,'readyState',{{configurable:true,get:()=>4}});
      Object.defineProperty(this,'status',{{configurable:true,get:()=>200}});
      Object.defineProperty(this,'responseText',{{configurable:true,get:()=>'{{}}'}});
      if (this.onreadystatechange) try{{this.onreadystatechange()}}catch(e){{}}
      if (this.onload) try{{this.onload()}}catch(e){{}}
    }} catch(e){{}}
  }};
}})();
</script>
<script src="file://{BDMS}"></script>
<script>
(async()=>{{
  try {{
    if (!window.bdms) {{ document.title='ERR'; document.body.innerText='no bdms'; return; }}
    window.bdms.init({{aid:2079,pageId:40236,paths:{{include:['/webcast/*']}},boe:false}});
    await new Promise(r=>setTimeout(r,1500));
    window.__urls=[];
    const url='https://webcast-pc.amemv.com/webcast/room/create/?'+window.__query;
    const x=new XMLHttpRequest(); x.open('POST',url);
    x.setRequestHeader('Content-Type','application/x-www-form-urlencoded; charset=UTF-8');
    x.send(window.__body||'');
    await new Promise(r=>setTimeout(r,300));
    const hit=window.__urls.find(u=>u.includes('a_bogus'))||'';
    const m=hit.match(/a_bogus=([^&]+)/);
    if(m){{ document.title='OK'; document.body.innerText=decodeURIComponent(m[1]); }}
    else {{ document.title='FAIL'; document.body.innerText=JSON.stringify(window.__urls); }}
  }} catch(e) {{ document.title='ERR'; document.body.innerText=String(e); }}
}})();
</script></body></html>"""
    html_path.write_text(html)
    ver = json.loads(urllib.request.urlopen(f"http://127.0.0.1:{PORT}/json/version").read())
    ws = websocket.create_connection(ver["webSocketDebuggerUrl"], timeout=20)
    mid = 0
    def send(method, params=None, session_id=None):
        nonlocal mid
        mid += 1
        msg = {{"id": mid, "method": method, "params": params or {{}}}}
        if session_id: msg["sessionId"] = session_id
        ws.send(json.dumps(msg))
        while True:
            r = json.loads(ws.recv())
            if r.get("id") == mid:
                return r
    r = send("Target.createTarget", {{"url": "about:blank"}})
    tid = r["result"]["targetId"]
    r = send("Target.attachToTarget", {{"targetId": tid, "flatten": True}})
    sid = r["result"]["sessionId"]
    send("Page.enable", session_id=sid)
    send("Runtime.enable", session_id=sid)
    send("Emulation.setUserAgentOverride",
         {{"userAgent": UA, "platform": "Windows", "acceptLanguage": "zh-CN"}}, session_id=sid)
    send("Page.navigate", {{"url": f"file://{html_path}"}}, session_id=sid)
    time.sleep(5)
    r = send("Runtime.evaluate",
             {{"expression": "JSON.stringify({title:document.title,body:document.body.innerText.slice(0,400)})",
              "returnByValue": True}}, session_id=sid)
    val = json.loads(r["result"]["result"]["value"])
    ws.close()
    if val.get("title") != "OK":
        raise SystemExit(f"sign fail: {val}")
    print(val["body"].strip())

proc = start()
try:
    gen(proc)
finally:
    proc.terminate()
    try: proc.wait(timeout=3)
    except Exception: proc.kill()
    subprocess.run(["pkill", "-f", f"remote-debugging-port={PORT}"], capture_output=True)
`, userAgent, query, body, bdms, chrome, cdpPort, cacheDir())

	if err := os.WriteFile(script, []byte(py), 0o700); err != nil {
		return "", err
	}
	// prefer python3
	python := "python3"
	if p, err := exec.LookPath("python3"); err == nil {
		python = p
	}
	cmd := exec.Command(python, script)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bdms sign: %w\n%s", err, truncate(string(out), 400))
	}
	// last non-empty line
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	ab := strings.TrimSpace(lines[len(lines)-1])
	if ab == "" || strings.HasPrefix(ab, "need ") || strings.HasPrefix(ab, "sign fail") {
		return "", fmt.Errorf("bdms sign bad output: %s", truncate(string(out), 300))
	}
	// sanity length
	if len(ab) < 80 {
		return "", fmt.Errorf("a_bogus too short: %q", ab)
	}
	_ = time.Second
	return ab, nil
}
