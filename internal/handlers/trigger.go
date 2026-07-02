package handlers

import (
	"context"
	"encoding/json"
	"net/http"
)

// triggerPage is a minimal self-contained control panel served at GET /trigger.
// It POSTs back to the same path so a missing-media search can be kicked off
// from the dashboard without wiring a full frontend. LAN/tailnet-only, like the
// rest of the concierge surfaces.
const triggerPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Search Missing</title>
<style>
 body{background:#0f172a;color:#e2e8f0;font-family:system-ui,sans-serif;
   display:flex;flex-direction:column;align-items:center;justify-content:center;
   min-height:100vh;margin:0;gap:1.25rem}
 h1{font-weight:600;font-size:1.25rem;margin:0}
 button{background:#2563eb;color:#fff;border:0;border-radius:.5rem;
   padding:.9rem 1.6rem;font-size:1rem;cursor:pointer}
 button:disabled{opacity:.5;cursor:progress}
 pre{background:#1e293b;padding:1rem;border-radius:.5rem;min-width:16rem;
   white-space:pre-wrap;word-break:break-word}
</style></head><body>
<h1>Search all missing episodes + movies</h1>
<button id="go" onclick="run()">Search Missing</button>
<pre id="out">Idle.</pre>
<script>
async function run(){
 const b=document.getElementById('go'),o=document.getElementById('out');
 b.disabled=true;o.textContent='Triggering…';
 try{const r=await fetch('trigger',{method:'POST'});o.textContent=JSON.stringify(await r.json(),null,2);}
 catch(e){o.textContent='Request failed: '+e;}
 finally{b.disabled=false;}
}
</script></body></html>`

// TriggerSearchHandler serves the control page on GET and, on POST, asks Sonarr
// and Radarr to search for every monitored-but-missing item. Both commands run
// asynchronously upstream; the response reports whether each was accepted.
func (b *Bot) TriggerSearchHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(triggerPage))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), b.Cfg.HTTPTimeout)
		defer cancel()

		out := struct {
			Sonarr string `json:"sonarr"`
			Radarr string `json:"radarr"`
		}{Sonarr: "queued", Radarr: "queued"}

		if err := b.Sonarr.SearchMissing(ctx); err != nil {
			b.Log.Warn("trigger: sonarr search-missing failed", "err", err)
			out.Sonarr = "error: " + err.Error()
		}
		if err := b.Radarr.SearchMissing(ctx); err != nil {
			b.Log.Warn("trigger: radarr search-missing failed", "err", err)
			out.Radarr = "error: " + err.Error()
		}
		b.Log.Info("trigger: search-missing", "sonarr", out.Sonarr, "radarr", out.Radarr)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}
