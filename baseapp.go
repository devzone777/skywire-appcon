/*
baseapp app for skywire visor
*/
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
	//"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/SkycoinProject/dmsg/cipher"
	"github.com/SkycoinProject/skycoin/src/util/logging"

	"github.com/SkycoinProject/skywire-mainnet/internal/netutil"
	"github.com/SkycoinProject/skywire-mainnet/pkg/app"
	"github.com/SkycoinProject/skywire-mainnet/pkg/app/appnet"
	"github.com/SkycoinProject/skywire-mainnet/pkg/routing"
	"github.com/SkycoinProject/skywire-mainnet/pkg/util/buildinfo"
)

const (
	appName = "baseapp"
	netType = appnet.TypeSkynet
	port    = routing.Port(44)
)

var addr = flag.String("addr", ":8004", "address to bind")
var r = netutil.NewRetrier(50*time.Millisecond, 5, 2)
var clientConfig, _ = app.ClientConfigFromEnv()

var (
	baseApp   *app.Client
	clientCh  chan string
	baseConns map[cipher.PubKey]net.Conn
	connsMu   sync.Mutex
	log       *logging.MasterLogger
)

type envVars struct {
    VisorPk string `json:"visor_pk"`
    AppServerAddr string `json:"app_server_addr"`
    AppKey string `json:"app_key"`
}


func main() {
	log = app.NewLogger(appName)
	flag.Parse()

	if _, err := buildinfo.Get().WriteTo(log.Writer()); err != nil {
		log.Printf("Failed to output build info: %v", err)
	}


	a, err := app.NewClient(logging.MustGetLogger(fmt.Sprintf("app_%s", appName)), clientConfig)
	if err != nil {
		log.Fatal("Setup failure: ", err)
	}
	defer a.Close()
	log.Println("Successfully created baseapp app")

	baseApp = a

	clientCh = make(chan string)
	defer close(clientCh)

	baseConns = make(map[cipher.PubKey]net.Conn)
	go listenLoop()



	r := mux.NewRouter()

	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("", get).Methods(http.MethodGet)
	api.HandleFunc("", post).Methods(http.MethodPost)
	api.HandleFunc("", put).Methods(http.MethodPut)
	//api.HandleFunc("", delete).Methods(http.MethodDelete)
	api.HandleFunc("/Env", env)

	log.Println("Serving BaseApp ENV Params on localhost:4444")
	log.Fatal(http.ListenAndServe(":4444", r))
}

func env(w http.ResponseWriter, r *http.Request) {
    
    s := fmt.Sprint(clientConfig)
    tl := strings.Trim(s, "{")
    tr := strings.Trim(tl, "}")
    f := strings.Fields(tr)
    VISOR_PK := f[0]
    APP_SERVER_ADDR := f[1]
    APP_KEY := f[2]

    vv := fmt.Sprintf("[{\"visor_pk\": %q", VISOR_PK)
    pp := fmt.Sprintf(",\"app_server_addr\": %q", APP_SERVER_ADDR)
    kk := fmt.Sprintf(",\"app_key\": %q}]", APP_KEY)
    rstr := fmt.Sprintln(vv, pp, kk)
    stringc := []byte(rstr)

    var jsona []envVars
    err := json.Unmarshal(stringc, &jsona)
    if err != nil {
        log.Fatal(err)
    }

    j, _ := json.MarshalIndent(jsona, "", " ")

    jsonb := string(j)

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)

    fmt.Fprintln(w, jsonb)
}

func get(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(`{"message": "get not used yet"}`))
}

func post(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    w.Write([]byte(`{"message": "post not used yet"}`))
}

func put(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusAccepted)
    w.Write([]byte(`{"message": "put not used yet"}`))
}

//func delete(w http.ResponseWriter, r *http.Request) {
    //w.Header().Set("Content-Type", "application/json")
    //w.WriteHeader(http.StatusOK)
    //w.Write([]byte(`{"message": "delete not used yet"}`))
//}

func listenLoop() {
	l, err := baseApp.Listen(netType, port)
	if err != nil {
		log.Printf("Error listening network %v on port %d: %v\n", netType, port, err)
		return
	}

	for {
		log.Println("Accepting baseapp conn...")
		conn, err := l.Accept()
		if err != nil {
			log.Println("Failed to accept conn:", err)
			return
		}
		log.Println("Accepted baseapp conn")

		raddr := conn.RemoteAddr().(appnet.Addr)
		connsMu.Lock()
		baseConns[raddr.PubKey] = conn
		connsMu.Unlock()
		log.Printf("Accepted skychat conn on %s from %s\n", conn.LocalAddr(), raddr.PubKey)

		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	raddr := conn.RemoteAddr().(appnet.Addr)
	for {
		buf := make([]byte, 32*1024)
		n, err := conn.Read(buf)
		if err != nil {
			log.Println("Failed to read packet:", err)
			raddr := conn.RemoteAddr().(appnet.Addr)
			connsMu.Lock()
			delete(baseConns, raddr.PubKey)
			connsMu.Unlock()
			return
		}

		clientData, err := json.Marshal(map[string]string{"sender": raddr.PubKey.Hex(), "message": string(buf[:n])})
		if err != nil {
			log.Printf("Failed to marshal json: %v", err)
		}
		select {
		case clientCh <- string(clientData):
			log.Printf("Received and sent to ui: %s\n", clientData)
		default:
			log.Printf("Received and trashed: %s\n", clientData)
		}
	}
}

