package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/rs/cors"
)

var net192 = false

const (
	scp_ack = "ACK"
	scp_err = "ERR"
)

var execpath string
var bf_default string = "bf000"

type Biofabrica_data struct {
	BFId         string
	BFName       string
	Status       string
	CustomerId   string
	CustomerName string
	Address      string
	SWVersion    string
	LatLong      [2]float64
	LastUpdate   string
	BFIP         string
}

var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

var bfs = []Biofabrica_data{{"bf000", "Modelo", "ERRO", "HA", "Hubio Agro", "", "1.2.15", [2]float64{-18.9236672, -48.1827026}, "", "192.168.0.23"},
	{"bf001", "Cerradao", "PRONTO", "UG", "Unigeo", "", "1.2.15", [2]float64{-10.9236672, -38.1827026}, "", "10.10.0.4"}}

func checkErr(err error) {
	if err != nil {
		log.Println("[SCP ERROR]", err)
	}
}

func scp_splitparam(param string, separator string) []string {
	scp_data := strings.Split(param, separator)
	if len(scp_data) < 1 {
		return nil
	}
	return scp_data
}

func test_file(filename string) bool {
	mf, err := os.Stat(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			checkErr(err)
		}
		return false
	}
	fmt.Println("DEBUG: Arquivo encontrado", mf.Name())
	return true
}

func get_bf_index(bf_id string) int {
	if len(bf_id) > 0 {
		for i, v := range bfs {
			if v.BFId == bf_id {
				return i
			}
		}
	}
	return -1
}

func scp_proxy(bfid string, r *http.Request, endpoint string) *http.Response {
	var wr http.ResponseWriter
	ind := get_bf_index(bfid)
	if ind < 0 {
		fmt.Println("ERROR SCP PROXY: Biofabrica nao encontrada", bfid)
		return nil
	}
	bf_url := fmt.Sprintf("http://%s:5000/%s", bfs[ind].BFIP, endpoint)

	log.Println(r.RemoteAddr, " ", r.Method, " ", r.URL)
	fmt.Println("request URI", r.RequestURI)
	fmt.Println("request URL", r.URL)
	fmt.Println("remote IP", r.RemoteAddr)
	fmt.Println("Schene", r.URL.Scheme)

	r.URL.Scheme = "http"
	r.URL.Path = fmt.Sprintf("%s:5000%s", bfs[ind].BFIP, r.RequestURI)
	r.RequestURI = ""
	r.RemoteAddr = fmt.Sprintf("%s:5000", bfs[ind].BFIP)
	client := &http.Client{}
	// delHopHeaders(r.Header)
	fmt.Println("remote IP", r.RemoteAddr)

	// clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	// if err == nil {
	// 	appendHostToXForwardHeader(r.Header, clientIP)
	// }
	// fmt.Println("Client IP", clientIP)
	// resp, err := client.Do(r)
	resp, err := client.Get(bf_url)
	if err != nil {
		checkErr(err)
		return nil
	}
	defer resp.Body.Close()
	// delHopHeaders(resp.Header)

	// copyHeader(wr.Header(), resp.Header)
	wr.WriteHeader(resp.StatusCode)
	io.Copy(wr, resp.Body)

	fmt.Println("DEBUG RES=", resp)
	// fmt.Println(res)
	// fmt.Println("rdata=", rdata)
	return resp
	// // fmt.Println(string(rdata))
	// json.Unmarshal(rdata, &last_biofabrica)
	// // fmt.Println(last_biofabrica)
	// fmt.Println("DEBUG CHECK LASTVERSION: Ajustando ultima versão para:", last_biofabrica.Version)
	// biofabrica.LastVersion = last_biofabrica.Version
}

func main_network(rw http.ResponseWriter, r *http.Request) {

	fmt.Printf("Req: %s %s\n", r.Host, r.URL.Path)
	endpoint := r.URL.Path

	switch r.Method {
	case "GET":
		fmt.Println(" METODO GET")
		var jsonStr []byte
		// var err error
		if endpoint == "/" {
			rw.Header().Set("Content-Type", "application/json")
			fmt.Println("acessando raiz")
			jsonStr, _ = json.Marshal(bfs)
			os.Stdout.Write(jsonStr)
			rw.Write([]byte(jsonStr))
		} else {
			params := scp_splitparam(endpoint, "/")
			fmt.Println("end=", params, len(params))
			if len(params) >= 2 {
				cmd := params[1]
				if cmd == "bf_default" {
					ind := get_bf_index(bf_default)
					if ind < 0 {
						rw.Write([]byte(scp_err))
					} else {
						jsonStr, _ = json.Marshal(bfs)
						// os.Stdout.Write(jsonStr)
						rw.Write([]byte(jsonStr))
					}
				} else {
					ind := get_bf_index(bf_default)
					if ind < 0 {
						fmt.Println("ERROR SCP PROXY: Biofabrica nao encontrada", bf_default)
						return
					}

					bf_endpoint := fmt.Sprintf("http://%s:5000%s", bfs[ind].BFIP, endpoint)
					req, err := http.NewRequest(r.Method, bf_endpoint, r.Body)
					if err != nil {
						checkErr(err)
						return
					}

					req.Header = r.Header.Clone()
					req.URL.RawQuery = r.URL.RawQuery
					client := http.Client{
						Timeout: 7 * time.Second,
					}

					reqData, err := httputil.DumpRequest(req, true)
					if err != nil {
						checkErr(err)
						return
					}
					req.URL.Scheme = "http"
					log.Println("Forward Request Data", len(string(reqData)))

					resp, err := client.Do(req)
					if err != nil {
						fmt.Println("ERRO no client.Do")
						checkErr(err)
						return
					}
					defer resp.Body.Close()

					//Get dump of our response
					respData, err := httputil.DumpResponse(resp, true)
					if err != nil {
						checkErr(err)
						return
					}

					log.Println("Forward Request Response", len(string(respData)))

					//Copy the response headers to the actual response. DO THIS BEFORE CALLING WRITEHEADER.
					for k, v := range resp.Header {
						rw.Header()[k] = v
					}

					//set the statuscode whatever we got from the response
					rw.WriteHeader(resp.StatusCode)

					//Copy the response body to the actual response
					_, err = io.Copy(rw, resp.Body)
					if err != nil {
						log.Println(err)
						rw.Write([]byte("error"))
						return
					}
				}
			}
		}
		// totem_id := r.URL.Query().Get("Id")
		//fmt.Println("bio_id =", bio_id)
		//fmt.Println()
		// if len(totem_id) > 0 {
		// 	cmd := "GET/" + scp_totem + "/" + totem_id + "/END"
		// 	jsonStr = []byte(scp_sendmsg_master(cmd))
		// } else {
		// 	cmd := "GET/" + scp_totem + "/END"
		// 	jsonStr = []byte(scp_sendmsg_master(cmd))
		// }
		//os.Stdout.Write(jsonStr)
		//jsonStr = []byte(scp_sendmsg_master(cmd))
		// os.Stdout.Write(jsonStr)
		// w.Write([]byte(jsonStr))

	case "PUT":

		fmt.Println(" METODO PUT")
		var jsonStr []byte
		if endpoint == "/" {
			rw.Header().Set("Content-Type", "application/json")
			fmt.Println("acessando raiz")
			jsonStr, _ = json.Marshal(bfs)
			os.Stdout.Write(jsonStr)
			rw.Write([]byte(jsonStr))
		} else {
			params := scp_splitparam(endpoint, "/")
			fmt.Println("end=", params, len(params))
			if len(params) >= 2 {
				cmd := params[1]
				if cmd == "bf_default" {
					bfid := r.FormValue("BFId")
					if len(bfid) > 0 {
						ind := get_bf_index(bfid)
						if ind >= 0 {
							bf_default = bfid
							rw.Write([]byte(scp_ack))
							return
						}
					}
					rw.Write([]byte(scp_err))

				} else if cmd == "bf_update" {
					bfid := r.FormValue("BFId")
					if len(bfid) > 0 {
						ind := get_bf_index(bfid)
						if ind >= 0 {
							raddr := r.RemoteAddr
							r_split := scp_splitparam(raddr, ":")
							if len(r_split) > 1 {
								bfs[ind].BFIP = r_split[0]
							} else {
								fmt.Println("ERROR SCP MAIN NETWORK: BFId enviou request com endereço IP invalido", bfid, raddr)
							}
							currentTime := time.Now()
							bfs[ind].LastUpdate = currentTime.Format("2017-09-07 17:06:06")
							fmt.Println("DEBUG SCP MAIN NETWORK: Atualizado bfid=", bfid, " >>", bfs[ind])
						} else {
							fmt.Println("ERROR SCP MAIN NETWORK: BFId invalido no bf_update", bfid)
						}
					} else {
						fmt.Println("ERROR SCP MAIN NETWORK: BFId não informado no bf_update")
					}
				} else {
					ind := get_bf_index(bf_default)
					if ind < 0 {
						fmt.Println("ERROR SCP PROXY: Biofabrica nao encontrada", bf_default)
						return
					}

					bf_endpoint := fmt.Sprintf("http://%s:5000%s", bfs[ind].BFIP, endpoint)
					req, err := http.NewRequest(r.Method, bf_endpoint, r.Body)
					if err != nil {
						checkErr(err)
						return
					}

					req.Header = r.Header.Clone()
					req.URL.RawQuery = r.URL.RawQuery
					client := http.Client{
						Timeout: 5 * time.Second,
					}

					reqData, err := httputil.DumpRequest(req, true)
					if err != nil {
						checkErr(err)
						return
					}
					req.URL.Scheme = "http"
					log.Println("Forward Request Data", len(string(reqData)))

					resp, err := client.Do(req)
					if err != nil {
						fmt.Println("ERRO no client.Do")
						checkErr(err)
						return
					}
					defer resp.Body.Close()

					//Get dump of our response
					respData, err := httputil.DumpResponse(resp, true)
					if err != nil {
						checkErr(err)
						return
					}

					log.Println("Forward Request Response", len(string(respData)))

					//Copy the response headers to the actual response. DO THIS BEFORE CALLING WRITEHEADER.
					for k, v := range resp.Header {
						rw.Header()[k] = v
					}

					//set the statuscode whatever we got from the response
					rw.WriteHeader(resp.StatusCode)

					//Copy the response body to the actual response
					_, err = io.Copy(rw, resp.Body)
					if err != nil {
						log.Println(err)
						rw.Write([]byte("error"))
						return
					}
				}
			}
		}

	case "POST":

		fmt.Println(" METODO POST chamado")
		// var jsonStr []byte
		if endpoint == "/bf_update" {
			var bf_agent Biofabrica_data
			err := json.NewDecoder(r.Body).Decode(&bf_agent)
			if err != nil {
				fmt.Println("ERROR SCP MAIN NETWORK: Erro ao decodificar dados enviados pelo Agent")
				checkErr(err)
				return
			}
			bfid := bf_agent.BFId
			if len(bfid) > 0 {
				ind := get_bf_index(bfid)
				if ind >= 0 {
					raddr := r.RemoteAddr
					r_split := scp_splitparam(raddr, ":")
					if len(r_split) > 1 {
						bfs[ind].BFIP = r_split[0]
					} else {
						fmt.Println("ERROR SCP MAIN NETWORK: BFId enviou request com endereço IP invalido", bfid, raddr)
					}
					currentTime := time.Now()
					bfs[ind].LastUpdate = currentTime.Format("2017-09-07 17:06:06")
					fmt.Println("DEBUG SCP MAIN NETWORK: Atualizado bfid=", bfid, " >>", bfs[ind])
				} else {
					fmt.Println("ERROR SCP MAIN NETWORK: BFId invalido no bf_update", bfid)
				}
			} else {
				fmt.Println("ERROR SCP MAIN NETWORK: BFId não informado no bf_update", r)
			}
		}
		rw.Write([]byte(scp_ack))
	}
	// case "PUT":
	// 	err := r.ParseForm()
	// 	if err != nil {
	// 		fmt.Println("ParseForm() err: ", err)
	// 		return
	// 	}
	// 	// fmt.Println("Post from website! r.PostFrom = ", r.PostForm)
	// 	// fmt.Println("Post Data", r.Form)
	// 	totem_id := r.FormValue("Id")
	// 	if len(totem_id) >= 0 {
	// 		pump := r.FormValue("Pump")
	// 		peris := r.FormValue("Perist")
	// 		valve := r.FormValue("Valve")
	// 		value_status := r.FormValue("Status")
	// 		// fmt.Println("Pump = ", pump)
	// 		// fmt.Println("Valve = ", valve)
	// 		// fmt.Println("Status = ", valve_status)
	// 		if pump != "" {
	// 			cmd := "PUT/" + scp_totem + "/" + totem_id + "/" + scp_dev_pump + "," + pump + "/END"
	// 			jsonStr := []byte(scp_sendmsg_master(cmd))
	// 			// os.Stdout.Write(jsonStr)
	// 			w.Write([]byte(jsonStr))
	// 		}
	// 		if valve != "" {
	// 			cmd := "PUT/" + scp_totem + "/" + totem_id + "/" + scp_dev_valve + "," + valve + "," + value_status + "/END"
	// 			jsonStr := []byte(scp_sendmsg_master(cmd))
	// 			// os.Stdout.Write(jsonStr)
	// 			w.Write([]byte(jsonStr))
	// 		}
	// 		if peris != "" {
	// 			cmd := "PUT/" + scp_totem + "/" + totem_id + "/" + scp_dev_peris + "," + peris + "," + value_status + "/END"
	// 			jsonStr := []byte(scp_sendmsg_master(cmd))
	// 			// os.Stdout.Write(jsonStr)
	// 			w.Write([]byte(jsonStr))
	// 		}
	// 	}

	// default:

	// }
}

func main() {

	net192 = test_file("/etc/scpd/scp_net192.flag")
	if net192 {
		fmt.Println("WARN:  EXECUTANDO EM NET192\n\n\n")
		execpath = "/home/paulo/scp-project/"
	} else {
		execpath = "/home/scpadm/scp-project/"
	}

	mux := http.NewServeMux()
	cors := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodPut,
			http.MethodPost,
			http.MethodGet,
		},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
	})

	mux.HandleFunc("/", main_network)

	// mux.HandleFunc("/ibc_view", ibc_view)

	// mux.HandleFunc("/totem_view", totem_view)

	// mux.HandleFunc("/biofabrica_view", biofabrica_view)

	// mux.HandleFunc("/simulator", biofactory_sim)

	// mux.HandleFunc("/config", set_config)

	// mux.HandleFunc("/wdpanel", withdraw_panel)

	handler := cors.Handler(mux)

	http.ListenAndServe(":7077", handler)
}
