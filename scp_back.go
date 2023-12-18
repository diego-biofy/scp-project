package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/rs/cors"
)

// var demo = false
var devmode = false
var net192 = false

// var testmode = false

const scp_err = "ERR"
const scp_ack = "ACK"
const scp_dev_pump = "PUMP"
const scp_dev_aero = "AERO"
const scp_dev_valve = "VALVE"
const scp_dev_peris = "PERIS"
const scp_dev_heater = "HEATER"
const scp_biofabrica = "BIOFABRICA"
const scp_totem = "TOTEM"
const scp_bioreactor = "BIOREACTOR"
const scp_ibc = "IBC"
const scp_wdpanel = "WDPANEL"
const scp_config = "CONFIG"

const scp_par_withdraw = "WITHDRAW"
const scp_par_getconfig = "GETCONFIG"
const scp_par_out = "OUT"
const scp_par_ph4 = "PH4"
const scp_par_ph7 = "PH7"
const scp_par_ph10 = "PH10"
const scp_par_calibrate = "CALIBRATE"
const scp_par_save = "SAVE"
const scp_par_restart = "RESTART"
const scp_par_testmode = "TESTMODE"
const scp_par_techmode = "TECHMODE"
const scp_par_deviceaddr = "DEVICEADDR"
const scp_par_screenaddr = "SCREENADDR"
const scp_par_linewash = "LINEWASH"
const scp_par_linecip = "LINECIP"
const scp_par_circulate = "CIRCULATE"
const scp_par_manydraw = "MANYDRAW"
const scp_par_manyout = "MANYOUT"
const scp_par_continue = "CONTINUE"
const scp_par_reconfigdev = "RECONFIGDEV"
const scp_par_resetdata = "RESETDATA"
const scp_par_stopall = "STOPALL"
const scp_par_upgrade = "SYSUPGRADE"
const scp_par_bfdata = "BFDATA"
const scp_par_loadbfdata = "LOADBFDATA"
const scp_par_restore = "RESTORE"
const scp_par_getph = "GETPH"
const scp_par_clenaperis = "CLEANPERIS"
const scp_par_setvolume = "SETVOLUME"

// const scp_par_version = "SYSVERSION"

const scp_sched = "SCHED"
const bio_nonexist = "NULL"
const bio_cip = "CIP"
const bio_loading = "CARREGANDO"
const bio_unloading = "ESVAZIANDO"
const bio_producting = "PRODUZINDO"
const bio_empty = "VAZIO"
const bio_done = "CONCLUIDO"
const bio_error = "ERRO"
const bio_max_valves = 8
const max_buf = 8192

// const execpath = "./"
const localconfig_path = "/etc/scpd/"

const vol_bioreactor = 2000
const vol_ibc = 4000
const overhead = 1.1
const max_bios = 36
const max_days = 60

// type Bioreact struct {
// 	BioreactorID string
// 	Status       string
// 	Organism     string
// 	Volume       uint32
// 	Level        uint8
// 	Pumpstatus   bool
// 	Aerator      bool
// 	Valvs        [8]int
// 	Temperature  float32
// 	PH           float32
// 	Step         [2]int
// 	Timeleft     [2]int
// 	Timetotal    [2]int
// }

// type IBC struct {
// 	IBCID      string
// 	Status     string
// 	Organism   string
// 	Volume     uint32
// 	Level      uint8const scp_biofabrica = "BIOFABRICA"
// 	Pumpstatus bool
// 	Valvs      [4]int
// }

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

type Organism struct {
	Index      string
	Code       string
	Orgname    string
	Orgtype    string
	Lifetime   int
	Prodvol    int
	Cultmedium string
	Timetotal  int
	Temprange  [3]string
	Aero       [3]int
	PH         [3]string
}

type BioList struct {
	OrganismName string
	Code         string
	Selected     bool
}

type Prodlist struct {
	Bioid  string
	Values []int
	Codes  []string
}

var orgs []Organism
var lastsched []Prodlist
var execpath string
var thisbf = Biofabrica_data{}

func checkErr(err error) {
	if err != nil {
		log.Println("[SCP ERROR]", err)
	}
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

func scp_splitparam(param string, separator string) []string {
	scp_data := strings.Split(param, separator)
	if len(scp_data) < 1 {
		return nil
	}
	return scp_data
}

func bio_to_code(bioname string) string {
	n := len(bioname)
	if n < 1 {
		return ""
	}
	biosplit := strings.Split(bioname, " ")
	nick := ""
	for _, k := range biosplit {
		nick += string(k[0])
	}
	return nick
}

func get_first_bio_available(prod [max_bios][max_days]int, maxbio int, maxday int) (int, int) {
	nbio := -1
	nday := -1
	for i := 0; i < maxbio; i++ {
		for j := 0; j < maxday; j++ {
			if prod[i][j] < 0 {
				if nday < 0 || j < nday {
					nday = j
					nbio = i
				}
			}
		}
	}
	return nbio, nday
}

func load_organisms(filename string) int {
	var totalrecords int
	file, err := os.Open(filename)
	if err != nil {
		checkErr(err)
		return -1
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		checkErr(err)
		return -1
	}
	orgs = make([]Organism, len(records))
	for k, r := range records {
		ind := r[0]
		code := r[1]
		name := r[2]
		otype := r[3]
		lifetime, _ := strconv.Atoi(strings.Replace(r[4], " ", "", -1))
		volume, _ := strconv.Atoi(strings.Replace(r[5], " ", "", -1))
		medium := strings.Replace(r[6], " ", "", -1)
		tottime, _ := strconv.Atoi(strings.Replace(r[7], " ", "", -1))
		temprange1 := strings.Replace(r[8], " ", "", -1)
		temprange2 := strings.Replace(r[9], " ", "", -1)
		temprange3 := strings.Replace(r[10], " ", "", -1)
		aero1, _ := strconv.Atoi(strings.Replace(r[11], " ", "", -1))
		aero2, _ := strconv.Atoi(strings.Replace(r[12], " ", "", -1))
		aero3, _ := strconv.Atoi(strings.Replace(r[13], " ", "", -1))
		ph1 := strings.Replace(r[14], " ", "", -1)
		ph2 := strings.Replace(r[15], " ", "", -1)
		ph3 := strings.Replace(r[16], " ", "", -1)
		org := Organism{ind, code, name, otype, lifetime, volume, medium, tottime, [3]string{temprange1, temprange2, temprange3}, [3]int{aero1, aero2, aero3}, [3]string{ph1, ph2, ph3}}
		organs[code] = org
		totalrecords = k
	}
	return totalrecords
}

func save_bf_data(filename string) int {
	filecsv, err := os.Create(filename)
	if err != nil {
		checkErr(err)
		return -1
	}
	defer filecsv.Close()
	n := 0
	csvwriter := csv.NewWriter(filecsv)
	s := fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%f,%f,%s,%s", thisbf.BFId, thisbf.BFName, thisbf.Status, thisbf.CustomerId,
		thisbf.CustomerName, thisbf.Address, thisbf.SWVersion, thisbf.LatLong[0], thisbf.LatLong[1],
		thisbf.LastUpdate, thisbf.BFIP)
	csvstr := scp_splitparam(s, ",")
	// fmt.Println("DEBUG SAVE", csvstr)
	err = csvwriter.Write(csvstr)
	if err != nil {
		checkErr(err)
	} else {
		n++
	}
	csvwriter.Flush()
	return n
}

func min_bio_sim(farmarea int, dailyarea int, orglist []BioList) (int, int, int, int, []Prodlist) {
	var total int
	var totalorg, totaltime int32
	var op, ot map[int]int32
	var o []int
	total = 0
	totalorg = 0
	totaltime = 0
	o = []int{}
	op = make(map[int]int32)
	ot = make(map[int]int32)

	for k, r := range orglist {
		if r.Selected {
			o = append(o, k)
			op[k] = int32(orgs[k].Prodvol * farmarea)
			totalorg += op[k]
			ot[k] = op[k] * int32(orgs[k].Timetotal)
			totaltime += ot[k]
			fmt.Println(orgs[k].Orgname, op[k], ot[k])
		}
	}
	fmt.Println("Volume total =", totalorg)
	fmt.Println("Tempo total =", totaltime)
	ndias := int(farmarea / dailyarea)
	fmt.Println("Numero dias =", ndias)
	total = int(math.Ceil(float64((float64(totaltime) * overhead) / float64(ndias*24*vol_bioreactor))))
	fmt.Println("Numero bioreatores =", total)
	fmt.Println("Organismos:", o)
	fmt.Println("Producao :", op)

	if ndias > max_days || total > max_bios {
		fmt.Println("numero maximo de dias ou bio excedido")
		return ndias, total, 0, 0, nil
	}
	var prodm [max_bios][max_days]int

	for i := 0; i < max_bios; i++ {
		for j := 0; j < max_days; j++ {
			prodm[i][j] = -1
		}
	}
	//	prodm = make(map[int][int]int)
	// i := 0
	d := 0
	b := 0
	// d0 := 0
	n := 0
	fday := -1
	haschange := true
	for d < ndias && haschange {
		//fmt.Println(prodm)
		haschange = false

		// d = 0
		// for d0 = 0; d0 < ndias; d0++ {
		// 	if prodm[b][d0] == 0 {
		// 		d = d0
		// 		break
		// 	}
		// }
		b, d = get_first_bio_available(prodm, total, ndias)
		if b < 0 || d < 0 {
			fmt.Println("Nao ha slot de producao disponivel")
			break
		}
		//fmt.Println("bio=", b, "dia=", d, "org=", n, " fday=", fday)
		for {
			if op[o[n]] > 0 {
				for i := 0; i < int(orgs[o[n]].Timetotal/24); i++ {
					//fmt.Print("dia=", d, " org=", n, " time=", orgs[o[n]].Timetotal, " prod=", op[o[n]])
					prodm[b][d] = o[n]
					proday := int32(math.Ceil(float64(vol_bioreactor*24) / float64(orgs[o[n]].Timetotal)))
					//fmt.Println(" proday=", proday)
					op[o[n]] -= proday
					d++
					haschange = true
				}
			}
			n++
			if n == len(o) {
				if fday < 0 {
					fday = d
				}
				n = 0
				if !haschange {
					break
				}
			}
			if haschange {
				break
			}
		}
		if d >= ndias {
			break
		}

	}

	//fmt.Println(prodm)

	max := 0
	v := make([]Prodlist, 0)
	for k, x := range prodm {
		var tmpcode []string
		tmpcode = []string{}
		var tmpnum []int
		tmpnum = []int{}
		if k < total {
			fmt.Printf("Bio%02d  ", k+1)
			for j, y := range x {
				if y >= 0 {
					fmt.Printf("%2d ", y)
					tmpcode = append(tmpcode, orgs[y].Code) // bio_to_code(orgs[y].Orgname)
					tmpnum = append(tmpnum, y)
					if j > max {
						max = j
					}
				}
			}
			fmt.Println()
			bioid := fmt.Sprintf("BIOR%02d", k+1)
			v = append(v, Prodlist{bioid, tmpnum, tmpcode})
		}
	}
	prodias := max + 1
	fmt.Println("Dias de Producao =", prodias)
	fmt.Println("Primeiro dia =", fday)
	//var jsonStr []byte
	//jsonStr, err := json.Marshal(prodm)
	//checkErr(err)
	//fmt.Println(prodm)
	//fmt.Println(v)
	//fmt.Println(jsonStr)

	// jsonStr, err := json.Marshal(v)
	// checkErr(err)
	// os.Stdout.Write(jsonStr)
	return ndias, total, prodias, fday, v
}

func scp_sendmsg_master(cmd string) string {

	ipc, err := net.Dial("unix", "/tmp/scp_master.sock")
	if err != nil {
		checkErr(err)
		return scp_err
	}
	defer ipc.Close()

	_, err = ipc.Write([]byte(cmd))
	if err != nil {
		checkErr(err)
		return scp_err
	}

	buf := make([]byte, max_buf)
	n, errf := ipc.Read(buf)
	if errf != nil {
		checkErr(err)
	}
	//fmt.Printf("recebido: %s\n", buf[:n])
	return string(buf[:n])
}

func ibc_view(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// fmt.Println("bio", ibc)
	//fmt.Println("Request", r)
	switch r.Method {
	case "GET":
		var jsonStr []byte
		ibc_id := r.URL.Query().Get("Id")
		//fmt.Println("ibc_id =", ibc_id)
		//fmt.Println()
		if len(ibc_id) > 0 {
			cmd := "GET/IBC/" + ibc_id + "/END"
			jsonStr = []byte(scp_sendmsg_master(cmd))
		} else {
			cmd := "GET/IBC/END"
			jsonStr = []byte(scp_sendmsg_master(cmd))
		}
		// os.Stdout.Write(jsonStr)
		w.Write([]byte(jsonStr))

	case "PUT":
		err := r.ParseForm()
		if err != nil {
			fmt.Println("ParseForm() err: ", err)
			return
		}
		// fmt.Println("Post from website! r.PostFrom = ", r.PostForm)
		// fmt.Println("Post Data", r.Form)
		// for i, d := range r.Form {
		// 	fmt.Println(i, d)
		// }
		ibc_id := r.FormValue("Id")
		if len(ibc_id) >= 0 {
			pump := r.FormValue("Pumpstatus")
			valve := r.FormValue("Valve")
			valve_status := r.FormValue("Status")
			withdraw := r.FormValue("Withdraw")
			outdev := r.FormValue("Out")
			b_pause := r.FormValue("Pause")
			b_stop := r.FormValue("Stop")
			b_start := r.FormValue("Start")
			orgcode := r.FormValue("OrgCode")
			recirc := r.FormValue("Recirculate")
			manydraw := r.FormValue("ManyDraw")
			manyout := r.FormValue("ManyOut")

			if manydraw != "" || manyout != "" {
				q := ""
				for i := 1; i <= 7; i++ {
					id_str := fmt.Sprintf("IBC%02d", i)
					out_str := r.FormValue(id_str)
					if len(out_str) > 0 {
						q += id_str + "=" + out_str + ","
					}
				}
				if len(q) > 0 {
					cmd := "PUT/IBC/ALL/" + scp_par_manydraw + "/" + q + "0/END"
					if manyout != "" {
						cmd = "PUT/IBC/ALL/" + scp_par_manyout + "/" + q + "0/END"
					}
					jsonStr := []byte(scp_sendmsg_master(cmd))
					w.Write([]byte(jsonStr))
				} else {
					w.Write([]byte(scp_err))
				}
			}

			if recirc != "" {
				recirc_time_str := r.FormValue("Time")
				if len(recirc_time_str) == 0 {
					recirc_time_str = "5"
				}
				cmd := "PUT/IBC/" + ibc_id + "/" + scp_par_circulate + "," + recirc + "," + recirc_time_str + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if pump != "" {
				cmd := "PUT/IBC/" + ibc_id + "/" + scp_dev_pump + "," + pump + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if valve != "" {
				cmd := "PUT/IBC/" + ibc_id + "/" + scp_dev_valve + "," + valve + "," + valve_status + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if outdev != "" {
				// fmt.Println("PAR OUT", outdev)
				cmd := "PUT/IBC/" + ibc_id + "/" + scp_par_out + "," + outdev + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if withdraw != "" {
				cmd := "PUT/IBC/" + ibc_id + "/" + scp_par_withdraw + "," + withdraw + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if b_start != "" {
				if orgcode != "" {
					cmd := "START/IBC/" + ibc_id + "/" + orgcode + "/END"
					jsonStr := []byte(scp_sendmsg_master(cmd))
					w.Write([]byte(jsonStr))
				} else {
					fmt.Println("ERROR IBC VIEW: Start faltando orgcode", r)
				}
			}
			if b_pause != "" {
				cmd := "PAUSE/IBC/" + ibc_id + "/" + b_pause + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if b_stop != "" {
				cmd := "STOP/IBC/" + ibc_id + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}

		}

	default:

	}
}

func bioreactor_view(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// fmt.Println("bio", bio)
	//fmt.Println("Request", r)
	switch r.Method {
	case "GET":
		var jsonStr []byte
		bio_id := r.URL.Query().Get("Id")
		//fmt.Println("bio_id =", bio_id)
		//fmt.Println()
		if len(bio_id) > 0 {
			cmd := "GET/BIOREACTOR/" + bio_id + "/END"
			jsonStr = []byte(scp_sendmsg_master(cmd))
		} else {
			cmd := "GET/BIOREACTOR/END"
			jsonStr = []byte(scp_sendmsg_master(cmd))
		}
		//os.Stdout.Write(jsonStr)
		w.Write([]byte(jsonStr))
	case "PUT":
		err := r.ParseForm()
		if err != nil {
			fmt.Println("ParseForm() err: ", err)
			return
		}
		//fmt.Println("Put from website! r.PostFrom = ", r.PostForm)
		//fmt.Println("Put Data", r.Form)
		// for i, d := range r.Form {
		// 	fmt.Println(i, d)
		// }
		bio_id := r.FormValue("Id")
		if len(bio_id) >= 0 {
			pump := r.FormValue("Pumpstatus")
			aero := r.FormValue("Aerator")
			valve := r.FormValue("Valve")
			peris := r.FormValue("Perist")
			b_pause := r.FormValue("Pause")
			b_stop := r.FormValue("Stop")
			b_start := r.FormValue("Start")
			orgcode := r.FormValue("OrgCode")
			value_status := r.FormValue("Status")
			withdraw := r.FormValue("Withdraw")
			outdev := r.FormValue("Out")
			recirc := r.FormValue("Recirculate")
			cleanperis := r.FormValue("CleanPeris")
			cont := r.FormValue("Continue")
			heater := r.FormValue("Heater")
			volume := r.FormValue("Volume")

			if b_start != "" {
				if orgcode != "" {
					vol_str := "2000"
					if volume != "" {
						vol_str = volume
					}
					cmd := "START/BIOREACTOR/" + bio_id + "/" + orgcode + "/" + vol_str + "/END"
					jsonStr := []byte(scp_sendmsg_master(cmd))
					w.Write([]byte(jsonStr))
				} else {
					fmt.Println("ERROR BIOREACTOR VIEW: Start faltando orgcode", r)
				}
			}

			if cont != "" {
				fmt.Println("CONTINUE PRESSIONADO ")
				cmd := "PUT/BIOREACTOR/" + bio_id + "/" + scp_par_continue + "," + cont + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if recirc != "" {
				recirc_time_str := r.FormValue("Time")
				if len(recirc_time_str) == 0 {
					recirc_time_str = "5"
				}
				cmd := "PUT/BIOREACTOR/" + bio_id + "/" + scp_par_circulate + "," + recirc + "," + recirc_time_str + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}

			if cleanperis != "" {
				clean_time_str := r.FormValue("Time")
				if len(clean_time_str) == 0 {
					clean_time_str = "10"
				}
				cmd := "PUT/BIOREACTOR/" + bio_id + "/" + scp_par_clenaperis + "," + cleanperis + "," + clean_time_str + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}

			if b_pause != "" {
				cmd := "PAUSE/BIOREACTOR/" + bio_id + "/" + b_pause + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if b_stop != "" {
				cmd := "STOP/BIOREACTOR/" + bio_id + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}

			if pump != "" {
				cmd := "PUT/BIOREACTOR/" + bio_id + "/" + scp_dev_pump + "," + pump + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if aero != "" {
				cmd := "PUT/BIOREACTOR/" + bio_id + "/" + scp_dev_aero + "," + aero + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if valve != "" {
				cmd := "PUT/BIOREACTOR/" + bio_id + "/" + scp_dev_valve + "," + valve + "," + value_status + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))

			}
			if heater != "" {
				cmd := "PUT/BIOREACTOR/" + bio_id + "/" + scp_dev_heater + "," + heater + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))

			}
			if outdev != "" {
				fmt.Println("PAR OUT", outdev)
				cmd := "PUT/BIOREACTOR/" + bio_id + "/" + scp_par_out + "," + outdev + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if withdraw != "" {
				cmd := "PUT/BIOREACTOR/" + bio_id + "/" + scp_par_withdraw + "," + withdraw + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}
			if peris != "" {
				cmd := "PUT/BIOREACTOR/" + bio_id + "/" + scp_dev_peris + "," + peris + "," + value_status + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
		}

	default:
		fmt.Fprintf(w, "Sorry, only GET and POST methods are supported.")
	}
	// fmt.Println()
	// fmt.Println()
}

func totem_view(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		var jsonStr []byte
		totem_id := r.URL.Query().Get("Id")
		//fmt.Println("bio_id =", bio_id)
		//fmt.Println()
		if len(totem_id) > 0 {
			cmd := "GET/" + scp_totem + "/" + totem_id + "/END"
			jsonStr = []byte(scp_sendmsg_master(cmd))
		} else {
			cmd := "GET/" + scp_totem + "/END"
			jsonStr = []byte(scp_sendmsg_master(cmd))
		}
		//os.Stdout.Write(jsonStr)
		//jsonStr = []byte(scp_sendmsg_master(cmd))
		// os.Stdout.Write(jsonStr)
		w.Write([]byte(jsonStr))

	case "PUT":
		err := r.ParseForm()
		if err != nil {
			fmt.Println("ParseForm() err: ", err)
			return
		}
		// fmt.Println("Post from website! r.PostFrom = ", r.PostForm)
		// fmt.Println("Post Data", r.Form)
		totem_id := r.FormValue("Id")
		if len(totem_id) >= 0 {
			pump := r.FormValue("Pump")
			peris := r.FormValue("Perist")
			valve := r.FormValue("Valve")
			value_status := r.FormValue("Status")
			// fmt.Println("Pump = ", pump)
			// fmt.Println("Valve = ", valve)
			// fmt.Println("Status = ", valve_status)
			if pump != "" {
				cmd := "PUT/" + scp_totem + "/" + totem_id + "/" + scp_dev_pump + "," + pump + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if valve != "" {
				cmd := "PUT/" + scp_totem + "/" + totem_id + "/" + scp_dev_valve + "," + valve + "," + value_status + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if peris != "" {
				cmd := "PUT/" + scp_totem + "/" + totem_id + "/" + scp_dev_peris + "," + peris + "," + value_status + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
		}

	default:

	}
}

func biofabrica_view(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		var jsonStr []byte
		cmd := "GET/BIOFABRICA/END"
		jsonStr = []byte(scp_sendmsg_master(cmd))
		// os.Stdout.Write(jsonStr)
		w.Write([]byte(jsonStr))

	case "PUT":
		err := r.ParseForm()
		if err != nil {
			fmt.Println("ParseForm() err: ", err)
			return
		}
		// fmt.Println("Post from website! r.PostFrom = ", r.PostForm)
		// fmt.Println("Post Data", r.Form)

		pump := r.FormValue("Pumpwithdraw")
		valve := r.FormValue("Valve")
		valve_status := r.FormValue("Status")
		linewash := r.FormValue("Linewash")
		lw_time := r.FormValue("Time")
		linecip := r.FormValue("LineCIP")
		// fmt.Println("Pumpwithdraw = ", pump)
		// fmt.Println("Valve = ", valve)
		// fmt.Println("Status = ", valve_status)
		if pump != "" {
			cmd := "PUT/" + scp_biofabrica + "/" + scp_dev_pump + "," + pump + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			// os.Stdout.Write(jsonStr)
			w.Write([]byte(jsonStr))
		}
		if valve != "" {
			cmd := "PUT/" + scp_biofabrica + "/" + scp_dev_valve + "," + valve + "," + valve_status + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			// os.Stdout.Write(jsonStr)
			w.Write([]byte(jsonStr))
		}
		if linewash != "" {
			time_str := "30"
			if len(lw_time) > 0 {
				time_str = lw_time
			}
			cmd := "START/" + scp_biofabrica + "/" + scp_par_linewash + "/" + linewash + "/" + time_str + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			// os.Stdout.Write(jsonStr)
			w.Write([]byte(jsonStr))
		}
		if linecip != "" {
			cmd := "START/" + scp_biofabrica + "/" + scp_par_linecip + "/" + linecip + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			// os.Stdout.Write(jsonStr)
			w.Write([]byte(jsonStr))
		}

	default:

	}
}

func set_config(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		err := r.ParseForm()
		if err != nil {
			fmt.Println("ParseForm() err: ", err)
			return
		}
		bioid := r.FormValue("BioId")
		ibcid := r.FormValue("IBCId")
		totemid := r.FormValue("TotemId")
		bfid := r.FormValue("BFId")

		fmt.Println("DEBUG SET CONFIG: GET com parametros: ", bioid, ibcid, totemid, bfid)

		if len(bioid) > 0 {
			fmt.Println("DEBUG SET CONFIG: GET", bioid)
			cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_getconfig + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			w.Write([]byte(jsonStr))
		}
		if len(ibcid) > 0 {
			fmt.Println("DEBUG SET CONFIG: GET", ibcid)
			cmd := scp_config + "/" + scp_ibc + "/" + ibcid + "/" + scp_par_getconfig + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			w.Write([]byte(jsonStr))
		}
		if len(totemid) > 0 {
			fmt.Println("DEBUG SET CONFIG: GET", totemid)
			cmd := scp_config + "/" + scp_totem + "/" + totemid + "/" + scp_par_getconfig + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			w.Write([]byte(jsonStr))
		}
		if len(bfid) > 0 {
			fmt.Println("DEBUG SET CONFIG: GET", bfid)
			cmd := ""
			if bfid == "CAD" {
				cmd = scp_config + "/" + scp_biofabrica + "/" + scp_par_bfdata + "/END"
			} else {
				cmd = scp_config + "/" + scp_biofabrica + "/" + scp_par_getconfig + "/END"
			}
			jsonStr := []byte(scp_sendmsg_master(cmd))
			w.Write([]byte(jsonStr))
		}

	case "POST":

		fmt.Println(" METODO POST chamado")
		var bf_agent Biofabrica_data
		err := json.NewDecoder(r.Body).Decode(&bf_agent)
		if err != nil {
			fmt.Println("ERROR SET CONFIG POST: Erro ao decodificar dados enviados pelo Agent")
			checkErr(err)
			w.Write([]byte(scp_err))
			return
		}
		fmt.Println("DEBUG SET CONFIG POST: Dados recebidos ", bf_agent)
		if len(bf_agent.BFId) < 3 {
			bf_agent.BFId = thisbf.BFId
		}
		if len(bf_agent.BFName) == 0 {
			bf_agent.BFName = thisbf.BFName
		}
		thisbf = bf_agent
		if save_bf_data(localconfig_path+"bf_data_new.csv") > 0 {
			fmt.Println("DEBUG SET CONFIG POST: Dados da Biofabrica gravados com sucesso")
		} else {
			fmt.Println("ERROR SET CONFIG POST: Falha ao gravar Dados da Biofabrica")
			w.Write([]byte(scp_err))
			return
		}
		cmd := scp_config + "/" + scp_biofabrica + "/" + scp_par_loadbfdata + "/END"
		jsonStr := []byte(scp_sendmsg_master(cmd))
		w.Write([]byte(jsonStr))

	case "PUT":
		err := r.ParseForm()
		if err != nil {
			fmt.Println("ParseForm() err: ", err)
			return
		}
		bioid := r.FormValue("BioId")
		ibcid := r.FormValue("IBCId")
		totemid := r.FormValue("TotemId")
		bfid := r.FormValue("BFId")
		devid := r.FormValue("Deviceid")
		devaddr := r.FormValue("Deviceaddr")
		scraddr := r.FormValue("Screenaddr")
		ph4 := r.FormValue("PH4")
		ph7 := r.FormValue("PH7")
		ph10 := r.FormValue("PH10")
		calibrate := r.FormValue("Calibrate")
		saveconfig := r.FormValue("SaveConfig")
		restart := r.FormValue("Restart")
		testm := r.FormValue("TestMode")
		techm := r.FormValue("TechMode")
		reconfigdev := r.FormValue("ReconfigDev")
		resetdata := r.FormValue("ResetData")
		stopall := r.FormValue("StopAll")
		upgrade := r.FormValue("Upgrade")
		restore := r.FormValue("Restore")
		getph := r.FormValue("GetPH")
		setvolume := r.FormValue("SetVolume")
		// sysversion := r.FormValue("Version")

		if len(bioid) > 0 {
			if stopall != "" {
				cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_stopall + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if devaddr != "" {
				cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_deviceaddr + "/" + devaddr + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if scraddr != "" {
				cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_screenaddr + "/" + scraddr + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if ph4 != "" {
				cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_ph4 + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}

			if ph7 != "" {
				cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_ph7 + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}

			if ph10 != "" {
				cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_ph10 + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}

			if calibrate != "" {
				cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_calibrate + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}

			if len(getph) > 0 {
				fmt.Println("DEBUG SET CONFIG: GET PH", bioid)
				cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_getph + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				w.Write([]byte(jsonStr))
			}

			if resetdata != "" {
				cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_resetdata + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}

			if restore != "" {
				cmd := scp_config + "/" + scp_bioreactor + "/" + bioid + "/" + scp_par_restore + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
		}

		if len(reconfigdev) > 0 {
			cmd := scp_config + "/" + scp_biofabrica + "/" + scp_par_reconfigdev + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			// os.Stdout.Write(jsonStr)
			w.Write([]byte(jsonStr))
		}

		if len(ibcid) > 0 {
			if stopall != "" {
				cmd := scp_config + "/" + scp_ibc + "/" + ibcid + "/" + scp_par_stopall + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if devaddr != "" {
				cmd := scp_config + "/" + scp_ibc + "/" + ibcid + "/" + scp_par_deviceaddr + "/" + devaddr + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if resetdata != "" {
				cmd := scp_config + "/" + scp_ibc + "/" + ibcid + "/" + scp_par_resetdata + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if setvolume != "" {
				cmd := scp_config + "/" + scp_ibc + "/" + ibcid + "/" + scp_par_setvolume + "/" + setvolume + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
		}

		if len(totemid) > 0 {
			if stopall != "" {
				cmd := scp_config + "/" + scp_totem + "/" + totemid + "/" + scp_par_stopall + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if devaddr != "" {
				cmd := scp_config + "/" + scp_totem + "/" + totemid + "/" + scp_par_deviceaddr + "/" + devaddr + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
		}

		if len(bfid) > 0 {
			if stopall != "" {
				cmd := scp_config + "/" + scp_biofabrica + "/" + scp_par_stopall + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if devaddr != "" {
				cmd := scp_config + "/" + scp_biofabrica + "/" + scp_par_deviceaddr + "/" + devid + "/" + devaddr + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if upgrade != "" {
				cmd := scp_config + "/" + scp_biofabrica + "/" + scp_par_upgrade + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			if resetdata != "" {
				cmd := scp_config + "/" + scp_biofabrica + "/" + scp_par_resetdata + "/END"
				jsonStr := []byte(scp_sendmsg_master(cmd))
				// os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
		}

		if saveconfig != "" {
			cmd := scp_config + "/" + scp_biofabrica + "/" + scp_par_save + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			// os.Stdout.Write(jsonStr)
			w.Write([]byte(jsonStr))
		}

		if restart != "" {
			cmd := scp_config + "/" + scp_biofabrica + "/" + scp_par_restart + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			// os.Stdout.Write(jsonStr)
			w.Write([]byte(jsonStr))
		}

		if testm != "" {
			cmd := scp_config + "/" + scp_biofabrica + "/" + scp_par_testmode + "/" + testm + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			// os.Stdout.Write(jsonStr)
			w.Write([]byte(jsonStr))
		}

		if techm != "" {
			cmd := scp_config + "/" + scp_biofabrica + "/" + scp_par_techmode + "/" + techm + "/END"
			jsonStr := []byte(scp_sendmsg_master(cmd))
			// os.Stdout.Write(jsonStr)
			w.Write([]byte(jsonStr))
		}

	default:

	}
}

func biofactory_sim(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	orgdata := make([]BioList, len(orgs))
	for k, r := range orgs {
		orgdata[k].OrganismName = r.Orgname
		orgdata[k].Code = r.Code
		orgdata[k].Selected = false
	}
	orgcip := BioList{"CIP", "CIP", false}
	orgdata = append(orgdata, orgcip)
	// fmt.Println("bio", bio)
	switch r.Method {
	case "GET":
		var jsonStr []byte
		jsonStr, err := json.Marshal(orgdata)
		checkErr(err)
		//os.Stdout.Write(jsonStr)
		w.Write([]byte(jsonStr))
	case "PUT":
		err := r.ParseForm()
		if err != nil {
			fmt.Println("ParseForm() err: ", err)
			return
		}
		exec_str := r.FormValue("Execute")
		exec_cmd, _ := strconv.ParseBool(exec_str)
		if exec_cmd {
			s_str := ""
			for _, s := range lastsched {
				for k, c := range s.Codes {
					seq := fmt.Sprintf("%d", k)
					s_str += s.Bioid + "," + seq + "," + c + "/"
				}
			}
			s_str += "END"
			cmd := scp_sched + "/" + scp_biofabrica + "/" + s_str
			fmt.Println("DEBUG SIM: to master", cmd)
			jsonStr := []byte(scp_sendmsg_master(cmd))
			os.Stdout.Write(jsonStr)
			w.Write([]byte(jsonStr))
		}
		farm_area_form := r.FormValue("Farmarea")
		farm_area, _ := strconv.Atoi(farm_area_form)
		daily_area_form := r.FormValue("Dailyarea")
		daily_area, _ := strconv.Atoi(daily_area_form)
		org_sel_form := r.FormValue("Organismsel")
		fmt.Println(farm_area, daily_area, org_sel_form)
		sels := []int{}
		err = json.Unmarshal([]byte(org_sel_form), &sels)
		//fmt.Println(sels)
		if (len(sels) >= 0) && (farm_area > daily_area) {
			for _, r := range sels {
				if r < len(orgdata) {
					orgdata[r].Selected = true
				} else {
					fmt.Println("Invalid Organism index")
				}
				//fmt.Println(i, r)
			}
			//fmt.Println("orgdata =", orgdata)
			var ndias, numbios, diasprod, primdia int
			ndias, numbios, diasprod, primdia, lastsched = min_bio_sim(farm_area, daily_area, orgdata)
			type Result struct {
				Totaldays        int
				Totalbioreactors int
				Totalprod        int
				Firstday         int
				Scheduler        []Prodlist
			}
			var ret = Result{ndias, numbios, diasprod, primdia, lastsched}
			jsonStr, err := json.Marshal(ret)
			checkErr(err)
			w.Write([]byte(jsonStr))
		}

	default:
		fmt.Fprintf(w, "Sorry, only GET and POST methods are supported.")
	}
	fmt.Println()
	fmt.Println()
}

func withdraw_panel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// fmt.Println("bio", bio)
	switch r.Method {
	case "GET":
		// var jsonStr []byte
		// jsonStr, err := json.Marshal(orgdata)
		// checkErr(err)
		//os.Stdout.Write(jsonStr)
		fmt.Println("Metodo GET para WDPANEL nao suportado")
		w.Write([]byte(scp_err))
		return
	case "PUT":
		err := r.ParseForm()
		if err != nil {
			fmt.Println("ParseForm() err: ", err)
			w.Write([]byte(scp_err))
			return
		}
		id_str := r.FormValue("Id")
		value_str := r.FormValue("Value")
		vol_inc := r.FormValue("VolInc")
		vol_dec := r.FormValue("VolDec")
		start := r.FormValue("Start")
		stop := r.FormValue("Stop")

		if len(id_str) == 0 {
			w.Write([]byte(scp_err))
			return
		}

		id_int, err := strconv.Atoi(id_str)
		if err != nil {
			checkErr(err)
			w.Write([]byte(scp_err))
			return
		}

		if len(value_str) > 0 {
			value_int, err := strconv.Atoi(value_str)
			if err != nil {
				checkErr(err)
				w.Write([]byte(scp_err))
				return
			}
			if value_int == 1 {
				s_str := fmt.Sprintf("SELECT,IBC%02d/END", id_int+1)
				cmd := scp_wdpanel + "/" + s_str
				fmt.Println("DEBUG WDPANEL: to master", cmd)
				jsonStr := []byte(scp_sendmsg_master(cmd))
				os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
			return
		}

		if len(vol_inc) > 0 {
			vol_inc_int, err := strconv.Atoi(vol_inc)
			if err != nil {
				checkErr(err)
				w.Write([]byte(scp_err))
				return
			}
			if vol_inc_int == 1 {
				s_str := fmt.Sprintf("INC,IBC%02d/END", id_int+1)
				cmd := scp_wdpanel + "/" + s_str
				fmt.Println("DEBUG WDPANEL: to master", cmd)
				jsonStr := []byte(scp_sendmsg_master(cmd))
				os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
		}

		if len(vol_dec) > 0 {
			vol_dec_int, err := strconv.Atoi(vol_dec)
			if err != nil {
				checkErr(err)
				w.Write([]byte(scp_err))
			}
			if vol_dec_int == 1 {
				s_str := fmt.Sprintf("DEC,IBC%02d/END", id_int+1)
				cmd := scp_wdpanel + "/" + s_str
				fmt.Println("DEBUG WDPANEL: to master", cmd)
				jsonStr := []byte(scp_sendmsg_master(cmd))
				os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
		}

		if len(start) > 0 {
			start_int, err := strconv.Atoi(start)
			if err != nil {
				checkErr(err)
				w.Write([]byte(scp_err))
				return
			}
			if start_int == 1 {
				s_str := fmt.Sprintf("START,IBC%02d/END", id_int+1)
				cmd := scp_wdpanel + "/" + s_str
				fmt.Println("DEBUG WDPANEL: to master", cmd)
				jsonStr := []byte(scp_sendmsg_master(cmd))
				os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
		}

		if len(stop) > 0 {
			stop_int, err := strconv.Atoi(stop)
			if err != nil {
				checkErr(err)
				w.Write([]byte(scp_err))
				return
			}
			if stop_int == 1 {
				s_str := fmt.Sprintf("STOP,IBC%02d/END", id_int+1)
				cmd := scp_wdpanel + "/" + s_str
				fmt.Println("DEBUG WDPANEL: to master", cmd)
				jsonStr := []byte(scp_sendmsg_master(cmd))
				os.Stdout.Write(jsonStr)
				w.Write([]byte(jsonStr))
			}
		}

	default:
		fmt.Fprintf(w, "Sorry, only GET and POST methods are supported.")
	}
	fmt.Println()
	fmt.Println()
	return
}

func main() {

	net192 = test_file("/etc/scpd/scp_net192.flag")
	if net192 {
		fmt.Println("WARN:  EXECUTANDO EM NET192\n\n\n")
		execpath = "/home/paulo/scp-project/"
	} else {
		execpath = "/home/scpadm/scp-project/"
	}
	devmode = test_file("")
	//scp_bio_init()
	if load_organisms(execpath+"organismos_conf.csv") < 0 {
		fmt.Println("Não foi possivel ler o arquivo de organismos")
		return
	}
	//fmt.Println(orgs)
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

	lastsched = make([]Prodlist, 0)

	mux.HandleFunc("/bioreactor_view", bioreactor_view)

	mux.HandleFunc("/ibc_view", ibc_view)

	mux.HandleFunc("/totem_view", totem_view)

	mux.HandleFunc("/biofabrica_view", biofabrica_view)

	mux.HandleFunc("/simulator", biofactory_sim)

	mux.HandleFunc("/config", set_config)

	mux.HandleFunc("/wdpanel", withdraw_panel)

	handler := cors.Handler(mux)

	http.ListenAndServe(":5000", handler)
}
