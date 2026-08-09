package webui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/usif/textui"
	"github.com/piotrnar/gocoin/client/usif/vcon"
)

const TextUIMaxWait = 15 * time.Second // how long we hold a polling request waiting for new output

type textui_resp struct {
	Seq   uint64
	Data  string
	Cmds  []string `json:",omitempty"`
	Error string   `json:",omitempty"`
}

// The virtual console gives a full control over the node, so keep it off in the server mode.
func textui_allowed() bool {
	return !common.CFG.WebUI.ServerMode && vcon.Enabled()
}

func json_textui(w http.ResponseWriter, r *http.Request) {
	if !textui_allowed() {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var resp textui_resp

	if len(r.Form["cmd"]) > 0 {
		// executing anything requires a matching session-id
		if !checksid(r) {
			new_session_id(w)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if !vcon.PostLine(r.Form["cmd"][0]) {
			resp.Error = "Command dropped - too many still pending"
		}
		resp.Seq = vcon.Seq()
	} else {
		var from uint64
		if len(r.Form["seq"]) > 0 {
			from, _ = strconv.ParseUint(r.Form["seq"][0], 10, 64)
		}
		var wait time.Duration
		if len(r.Form["wait"]) > 0 {
			wait = TextUIMaxWait
		}
		var data []byte
		data, resp.Seq = vcon.Fetch(from, wait)
		resp.Data = string(data)
	}

	if len(r.Form["cmds"]) > 0 {
		resp.Cmds = textui.Commands()
	}

	bx, er := json.Marshal(&resp)
	if er != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header()["Content-Type"] = []string{"application/json"}
	w.Write(bx)
}
