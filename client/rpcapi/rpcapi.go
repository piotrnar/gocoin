package rpcapi

// test it with:
// curl --user someuser:somepass --data-binary '{"method":"Arith.Add","params":[{"A":7,"B":1}],"id":0}' -H 'content-type: text/plain;' http://127.0.0.1:8222/

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/piotrnar/gocoin/client/common"
)

type RpcError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type RpcResponse struct {
	Id     interface{} `json:"id"`
	Result interface{} `json:"result"`
	Error  interface{} `json:"error"`
}

type RpcCommand struct {
	Id     interface{} `json:"id"`
	Params interface{} `json:"params"`
	Method string      `json:"method"`
}

func process_rpc(b []byte) (out []byte) {
	os.WriteFile("rpc_cmd.json", b, 0777)
	ex_cmd := exec.Command("C:\\Tools\\DEV\\Git\\mingw64\\bin\\curl.EXE",
		"--user", "gocoinrpc:gocoinpwd", "--data-binary", "@rpc_cmd.json", "http://127.0.0.1:18332/")
	out, _ = ex_cmd.Output()
	return
}

func my_handler(w http.ResponseWriter, r *http.Request) {
	u, p, ok := r.BasicAuth()
	if !ok {
		println("No HTTP Authentication data")
		return
	}
	if u != common.CFG.RPC.Username {
		println("HTTP Authentication: bad username")
		return
	}
	if p != common.CFG.RPC.Password {
		println("HTTP Authentication: bad password")
		return
	}
	//fmt.Println("========================handler", r.Method, r.URL.String(), u, p, ok, "=================")
	b, e := io.ReadAll(r.Body)
	if e != nil {
		println(e.Error())
		return
	}

	var RpcCmd RpcCommand
	jd := json.NewDecoder(bytes.NewReader(b))
	jd.UseNumber()
	e = jd.Decode(&RpcCmd)
	if e != nil {
		println(e.Error())
	}

	var resp RpcResponse
	resp.Id = RpcCmd.Id
	//println("------------------------------ RPC command:", RpcCmd.Method, "---------------------------------------------")
	switch RpcCmd.Method {
	case "getblocktemplate":
		var resp_my RpcGetBlockTemplateResp
		GetNextBlockTemplate(&resp_my.Result)

		if false {
			var resp_ok RpcGetBlockTemplateResp
			bitcoind_result := process_rpc(b)
			//ioutil.WriteFile("getblocktemplate_resp.json", bitcoind_result, 0777)

			//fmt.Print("getblocktemplate...")

			jd = json.NewDecoder(bytes.NewReader(bitcoind_result))
			jd.UseNumber()
			jd.Decode(&resp_ok)

			if resp_my.Result.PreviousBlockHash != resp_ok.Result.PreviousBlockHash {
				println("satoshi @", resp_ok.Result.PreviousBlockHash, resp_ok.Result.Height)
				println("gocoin  @", resp_my.Result.PreviousBlockHash, resp_my.Result.Height)
			} else {
				println(".", len(resp_my.Result.Transactions), resp_my.Result.Coinbasevalue)
				if resp_my.Result.Mintime != resp_ok.Result.Mintime {
					println("\007Mintime:", resp_my.Result.Mintime, resp_ok.Result.Mintime)
				}
				if resp_my.Result.Bits != resp_ok.Result.Bits {
					println("\007Bits:", resp_my.Result.Bits, resp_ok.Result.Bits)
				}
				if resp_my.Result.Target != resp_ok.Result.Target {
					println("\007Target:", resp_my.Result.Target, resp_ok.Result.Target)
				}
			}
		}

		b, _ = json.Marshal(&resp_my)
		//os.WriteFile("json/"+RpcCmd.Method+"_resp_my.json", b, 0777)
		w.Write(append(b, 0x0a))
		//println(" ... ", string(b))
		return

	case "getwork":
		var resp_my RpcGetWorkResp
		//println("geting work...", DO_SEGWIT, WAIT_FOR_SECONDS, DO_NOT_SUBMIT)
		switch uu := RpcCmd.Params.(type) {
		case []interface{}:
			if len(uu) >= 1 {
				if currently_worked_block == nil {
					println("work submited, but no work in progress")
					return
				}
				d, err := hex.DecodeString(uu[0].(string))
				if err != nil {
					println(err.Error())
					return
				}
				swap32(d)
				currently_worked_block.Raw = d[:80]
				SubmitWork(currently_worked_block)
				currently_worked_block = nil
				resp.Result = true
				goto send_response
			}
		default:
			println("***something else")
		}
		GetWork(&resp_my)
		b, _ = json.Marshal(&resp_my)
		w.Write(append(b, 0x0a))
		//println(" ... ", string(b))
		return

	case "getmininginfo":
		//println("getmininginfo...")
		var rm RpcGetMiningInfoResp
		rm.Result = mining_info
		b, _ = json.Marshal(&rm)
		w.Write(append(b, 0x0a))
		//println(" ... ", string(b))
		return

	case "validateaddress":
		switch uu := RpcCmd.Params.(type) {
		case []interface{}:
			if len(uu) == 1 {
				resp.Result = ValidateAddress(uu[0].(string))
			}
		default:
			println("unexpected type", uu)
		}

	case "submitblock":
		println("_________________________SH__________________________________")
		//os.WriteFile("submitblock.json", b, 0777)
		SubmitBlock(&RpcCmd, &resp, b)

	default:
		fmt.Println("Method:", RpcCmd.Method, len(b))
		//w.Write(bitcoind_result)
		resp.Error = RpcError{Code: -32601, Message: "Method not found"}
	}

send_response:
	b, e = json.Marshal(&resp)
	if e != nil {
		println("json.Marshal(&resp):", e.Error())
	}

	//ioutil.WriteFile(RpcCmd.Method+"_resp.json", b, 0777)
	w.Write(append(b, 0x0a))
}

func StartServer(port uint32) {
	fmt.Println("Starting RPC server at port", port)
	mux := http.NewServeMux()
	mux.HandleFunc("/", my_handler)
	http.ListenAndServe(fmt.Sprint(":", port), mux)
}
