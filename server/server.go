package server

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/CaptainCore/captaincore/version"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

const letternumberBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var db *gorm.DB
var err error
var config Config
var debug bool

//go:embed *.webp
var staticFiles embed.FS

// var clients = make(map[*websocket.Conn]bool) // connected clients
type Client struct {
	Token string
	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

var clients []Client

var upgrader = websocket.Upgrader{
	// /ws authenticates on the per-task token carried in the first frame, not
	// on a cookie, so a cross-site connection has nothing to replay. Browsers
	// are still kept out by default: only a same-host page, or an origin named
	// in CAPTAINCORE_SERVER_ORIGINS, may upgrade. Non-browser clients (the
	// Manager) send no Origin header and are unaffected.
	CheckOrigin:     checkWebsocketOrigin,
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// checkWebsocketOrigin allows same-host and explicitly allow-listed origins.
func checkWebsocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // not a browser
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	if originAllowed(origin, config.Origins, os.Getenv("CAPTAINCORE_SERVER_ORIGINS")) {
		return true
	}
	log.Println("websocket upgrade refused for origin:", origin)
	return false
}

// originAllowed matches origin against the config-file list and the
// comma-separated environment list, ignoring case and trailing slashes.
func originAllowed(origin string, fromConfig []string, fromEnv string) bool {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	candidates := append([]string{}, fromConfig...)
	candidates = append(candidates, strings.Split(fromEnv, ",")...)
	for _, allowed := range candidates {
		allowed = strings.TrimRight(strings.TrimSpace(allowed), "/")
		if allowed != "" && strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

type httpHandlerFunc func(http.ResponseWriter, *http.Request)

const (
	htmlIndexTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>CaptainCore</title>
<link rel="icon" type="image/webp" href="/assets/icon-32.webp" sizes="32x32">
<link rel="icon" type="image/webp" href="/assets/icon-192.webp" sizes="192x192">
<link rel="apple-touch-icon" href="/assets/icon-180.webp">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Hanken+Grotesk:wght@500;700;800&display=swap">
<style>
:root{--accent:#0E8A80;--text:#15181D;--text-2:#565C66;--bg:#F5F7FA;--surface:#FFFFFF;--border:#E3E7EE}
@media (prefers-color-scheme:dark){:root{--accent:#3FC5B8;--text:#E9ECF1;--text-2:#A3ACB9;--bg:#0B0E13;--surface:#141922;--border:#242C39}}
*{box-sizing:border-box}
html,body{height:100%%}
body{margin:0;display:flex;align-items:center;justify-content:center;background:var(--bg);color:var(--text);font-family:"Hanken Grotesk",system-ui,-apple-system,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;-webkit-font-smoothing:antialiased}
main{text-align:center;padding:48px 24px}
.mark{width:96px;height:96px;display:block;margin:0 auto 20px}
h1{margin:0;font-size:40px;font-weight:800;letter-spacing:-0.01em;line-height:1.1}
.sub{margin:10px 0 0;color:var(--text-2);font-size:16px;font-weight:500}
.ver{display:inline-block;margin-left:6px;padding:2px 9px;border:1px solid var(--border);border-radius:999px;background:var(--surface);font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:13px;color:var(--text-2)}
nav{margin-top:32px;display:flex;gap:24px;justify-content:center;flex-wrap:wrap}
nav a{color:var(--accent);text-decoration:none;font-weight:700;font-size:15px;padding-bottom:2px;border-bottom:1.5px solid transparent}
nav a:hover{border-bottom-color:var(--accent)}
</style>
</head>
<body>
<main>
<picture>
<source srcset="/assets/mark-256-white.webp" media="(prefers-color-scheme: dark)">
<img class="mark" src="/assets/mark-256.webp" width="96" height="96" alt="">
</picture>
<h1>CaptainCore</h1>
<p class="sub">CLI server <span class="ver">v%s</span></p>
<nav>
<a href="https://captaincore.io">captaincore.io</a>
<a href="https://captaincore.io/docs/">Docs</a>
<a href="https://github.com/CaptainCore/captaincore">GitHub</a>
</nav>
</main>
</body>
</html>`
)

// Define our message object
type SocketRequest struct {
	Token  string `json:"token"`
	Action string `json:"action"`
}

type SocketResponse struct {
	Token  string `json:"token"`
	TaskID string `json:"task_id"`
}

type Config struct {
	Tokens []struct {
		CaptainID string `json:"captain_id"`
		Token     string `json:"token"`
	} `json:"tokens"`
	Servers []struct {
		Name     string `json:"name"`
		Address  string `json:"address"`
		Requires []struct {
			Command string `json:"command"`
		} `json:"requires"`
	} `json:"servers"`
	Host    string `json:"host"`
	Port    string `json:"port"`
	SSLMode string `json:"ssl_mode"`
	// Origins lists dashboard origins allowed to open the websocket, written by
	// `captaincore connect` from the Manager's dashboard URL. Merged with
	// CAPTAINCORE_SERVER_ORIGINS at check time.
	Origins []string `json:"origins"`
}

type Task struct {
	gorm.Model
	CaptainID int
	ProcessID int
	Command   string
	Status    string
	Response  string
	Origin    string
	Token     string `gorm:"index"`
	// ArgsJSON persists the argv for the new array protocol so queued/resumed
	// tasks reconstruct exactly (the human-readable Command stays display-only).
	ArgsJSON string `json:"-"`
	// Args and Payload are decoded from the request body only (never persisted).
	Args    []string `json:"args" gorm:"-"`
	Payload string   `json:"payload" gorm:"-"`
}

type taskWithProgress struct {
	Task
	Progress json.RawMessage `json:"progress,omitempty"`
}

type Origin struct {
	ID     string
	Server string
	Token  string
}

func LoadConfiguration(file string) Config {
	var c Config
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&c)
	return c
}

func fetchCaptainID(t string, r *http.Request) string {
	// Constant-time compare: this maps a token onto the tenant every handler
	// scopes its queries by, so a byte-at-a-time timing difference would leak
	// one tenant's token to another.
	captainID := "0"
	for _, v := range config.Tokens {
		if subtle.ConstantTimeCompare([]byte(v.Token), []byte(t)) == 1 {
			captainID = v.CaptainID
		}
	}
	return captainID
}

func fetchToken(captainID string) string {
	for _, v := range config.Tokens {
		if v.CaptainID == captainID {
			return v.Token
		}
	}
	return "0"
}

func generateToken() string {
	n := 48
	output := make([]byte, n)
	// We will take n bytes, one byte for each character of output.
	randomness := make([]byte, n)
	// read all random
	_, err := rand.Read(randomness)
	if err != nil {
		panic(err)
	}
	l := len(letternumberBytes)
	// fill output
	for pos := range output {
		// get random item
		random := uint8(randomness[pos])
		// random % 64
		randomPos := random % uint8(l)
		// put into output
		output[pos] = letternumberBytes[randomPos]
	}
	o := string(output)
	return o
}

func allTasks(w http.ResponseWriter, r *http.Request) {
	var tasks []Task
	vars := mux.Vars(r)
	token := r.Header.Get("token")
	captainID := fetchCaptainID(token, r)
	page, _ := strconv.Atoi(vars["page"])
	if page > 0 {
		offset := page * 10
		db.Offset(offset).Limit(10).Order("created_at desc").Where("captain_id = ?", captainID).Find(&tasks)
	} else {
		db.Limit(10).Order("created_at desc").Where("captain_id = ?", captainID).Find(&tasks)
	}

	json.NewEncoder(w).Encode(tasks)
}

func newRun(w http.ResponseWriter, r *http.Request) {
	var task Task
	json.NewDecoder(r.Body).Decode(&task)
	token := r.Header.Get("token")
	randomToken := generateToken()
	captainID := fetchCaptainID(token, r)
	if captainID == "0" {
		http.Error(w, "401 - Unauthorized", http.StatusUnauthorized)
		return
	}

	task.Status = "Started"
	task.CaptainID, err = strconv.Atoi(captainID)
	task.Token = randomToken

	prepareArgvTask(&task)
	db.Create(&task)

	// Starts running CaptainCore command
	head, args := buildExec(task, captainID)
	response := runCommand(head, args, task)
	fmt.Fprint(w, response)
}

func newRunStream(w http.ResponseWriter, r *http.Request) {
	var task Task
	json.NewDecoder(r.Body).Decode(&task)
	token := r.Header.Get("token")
	randomToken := generateToken()
	captainID := fetchCaptainID(token, r)
	if captainID == "0" {
		http.Error(w, "401 - Unauthorized", http.StatusUnauthorized)
		return
	}

	task.Status = "Started"
	task.CaptainID, err = strconv.Atoi(captainID)
	task.Token = randomToken

	prepareArgvTask(&task)
	db.Create(&task)

	// Starts running CaptainCore command
	head, args := buildExec(task, captainID)
	runStreamCommand(w, head, args, task)
}

func newBackground(w http.ResponseWriter, r *http.Request) {
	var task Task
	json.NewDecoder(r.Body).Decode(&task)
	token := r.Header.Get("token")
	randomToken := generateToken()
	captainID := fetchCaptainID(token, r)
	if captainID == "0" {
		http.Error(w, "401 - Unauthorized", http.StatusUnauthorized)
		return
	}

	task.Status = "Started"
	task.CaptainID, err = strconv.Atoi(captainID)
	task.Token = randomToken

	// Legacy string protocol: extract an inline --payload='...' to a file.
	if len(task.Args) == 0 {
		pattern := `(--payload='.+')`
		payload := regexp.MustCompile(pattern).FindString(task.Command)

		if len(payload) >= 1 {
			log.Println("Payload found, writing to file.")
			task.Command = strings.Replace(task.Command, payload, task.Token, -1)

			pattern_data := `--payload='(.+)'`
			payload_data := regexp.MustCompile(pattern_data).FindStringSubmatch(payload)
			writePayloadFile(task.Token, payload_data[1])
		}
	}

	prepareArgvTask(&task)
	db.Create(&task)

	// Starts running CaptainCore command
	head, args := buildExec(task, captainID)
	go runCommand(head, args, task)
	taskID := strconv.FormatUint(uint64(task.ID), 10)
	response := "{ \"task_id\" : " + taskID + ", \"token\" : \"" + task.Token + "\" }"
	fmt.Fprint(w, response)
}

func newTask(w http.ResponseWriter, r *http.Request) {
	var task Task
	json.NewDecoder(r.Body).Decode(&task)
	token := r.Header.Get("token")
	randomToken := generateToken()
	captainID := fetchCaptainID(token, r)
	if captainID == "0" {
		http.Error(w, "401 - Unauthorized", http.StatusUnauthorized)
		return
	}
	task.Status = "Queued"
	task.CaptainID, err = strconv.Atoi(captainID)
	task.Token = randomToken

	// Legacy string protocol: extract an inline --payload='...' to a file.
	if len(task.Args) == 0 {
		pattern := `(--payload='.+')`
		payload := regexp.MustCompile(pattern).FindString(task.Command)

		if len(payload) >= 1 {
			log.Println("Payload found, writing to file.")
			task.Command = strings.Replace(task.Command, payload, task.Token, -1)

			pattern_data := `--payload='(.+)'`
			payload_data := regexp.MustCompile(pattern_data).FindStringSubmatch(payload)
			writePayloadFile(task.Token, payload_data[1])
		}
	}

	prepareArgvTask(&task)
	db.Create(&task)
	taskID := strconv.FormatUint(uint64(task.ID), 10)
	response := "{ \"task_id\" : " + taskID + ", \"token\" : \"" + randomToken + "\" }"
	fmt.Fprint(w, response)
}

func WriteToFile(filename string, data string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.WriteString(file, data)
	if err != nil {
		return err
	}
	return file.Sync()
}

func deleteTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	token := r.Header.Get("token")
	captainID := fetchCaptainID(token, r)

	var task Task
	db.Where("id = ?", id).Where("captain_id = ?", captainID).Find(&task)

	if task.ID == 0 {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	if task.Status == "Started" && task.ProcessID > 0 {
		killCommand(task)
	}

	task.Status = "Cancelled"
	db.Save(&task)

	fmt.Fprintf(w, "Successfully Cancelled Task")
}

func streamTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	token := r.Header.Get("token")
	captainID := fetchCaptainID(token, r)

	var task Task
	db.Where("id = ?", id).Where("captain_id = ?", captainID).Find(&task)

	if task.ID == 0 {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	home, _ := os.UserHomeDir()
	lastData := ""

	type streamEvent struct {
		Status   string          `json:"status"`
		Progress json.RawMessage `json:"progress,omitempty"`
	}

	sendEvent := func(event streamEvent) {
		if data, err := json.Marshal(event); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		db.First(&task, task.ID)

		if task.Status == "Completed" || task.Status == "Cancelled" {
			sendEvent(streamEvent{Status: strings.ToLower(task.Status)})
			return
		}

		if task.ProcessID > 0 {
			progressPath := filepath.Join(home, ".captaincore", "data", "task-progress", strconv.Itoa(task.ProcessID)+".json")
			if data, err := os.ReadFile(progressPath); err == nil && json.Valid(data) {
				dataStr := string(data)
				if dataStr != lastData {
					lastData = dataStr
					sendEvent(streamEvent{Status: "started", Progress: json.RawMessage(data)})
				}
			}
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func viewTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	token := r.Header.Get("token")
	captainID := fetchCaptainID(token, r)

	var task Task
	db.Where("id = ?", id).Where("captain_id = ?", captainID).Find(&task)

	// Check for task progress file when task is running
	if task.Status == "Started" && task.ProcessID > 0 {
		home, err := os.UserHomeDir()
		if err == nil {
			progressPath := filepath.Join(home, ".captaincore", "data", "task-progress", strconv.Itoa(task.ProcessID)+".json")
			if data, err := os.ReadFile(progressPath); err == nil && json.Valid(data) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(taskWithProgress{
					Task:     task,
					Progress: json.RawMessage(data),
				})
				return
			}
		}
	}

	json.NewEncoder(w).Encode(task)
}

func updateTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	token := r.Header.Get("token")
	captainID := fetchCaptainID(token, r)

	var task Task
	db.Where("id = ?", id).Where("captain_id = ?", captainID).Find(&task)

	// Find() leaves a zero-value struct when nothing matched, and Save() on a
	// zero primary key INSERTs — so without this a PUT for someone else's task
	// id would silently create a new row.
	if task.ID == 0 {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	task.Status = "Completed"
	db.Save(&task)

	fmt.Fprintf(w, "Successfully Updated Task")
}

// assetHandler serves the embedded brand files behind /assets/<name>.
func assetHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	data, err := staticFiles.ReadFile(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(name)))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// A failed upgrade (e.g. a plain GET with no websocket headers) must not
		// take down the whole daemon — log and return instead of log.Fatal.
		log.Println("websocket upgrade failed:", err)
		return
	}
	// Clear any read/write deadline inherited from the http.Server so the
	// long-lived websocket isn't severed mid-command. Zero time = no deadline.
	conn.SetReadDeadline(time.Time{})
	conn.SetWriteDeadline(time.Time{})
	// Make sure we close the connection when the function returns
	defer conn.Close()

	newClient := Client{Token: "", conn: conn, send: make(chan []byte, 256)}
	clients = append(clients, newClient)
	log.Printf("Successfully established connection (%d client(s))", len(clients))

	for {
		data := SocketRequest{}
		err := conn.ReadJSON(&data)
		if err != nil {
			log.Println("Client disconnected:", err)
			// Find current connection and remove from clients
			for i := 0; i < len(clients); i++ {
				if clients[i].conn == conn {
					clients = append(clients[:i], clients[i+1:]...)
					i--
				}
			}
			break
		}

		var task Task
		db.Where("token = ?", data.Token).Find(&task)

		// Refuse connection if token not valid
		if task.Token == "" {
			// Find current connection and remove from clients
			for i := 0; i < len(clients); i++ {
				if clients[i].conn == conn {
					clients = append(clients[:i], clients[i+1:]...)
					i-- // form the remove item index to start iterate next item
				}
			}
			break
		}

		// Find current connection and update Token
		for i := 0; i < len(clients); i++ {
			if clients[i].conn == conn {
				clients[i].Token = data.Token
				break
			}
		}

		// Execute job if requested
		if data.Action == "start" {
			captainID := strconv.Itoa(task.CaptainID)
			// buildExec reconstructs argv from the persisted ArgsJSON (new
			// protocol) or tokenizes the stored Command (legacy).
			head, args := buildExec(task, captainID)
			go runCommand(head, args, task)
		}
		if data.Action == "listen" {
			captainID := strconv.Itoa(task.CaptainID)
			head, args := parseCommandString("captaincore running listen --captain-id=" + captainID)
			go runCommand(head, args, task)
		}
		if data.Action == "kill" {
			go killCommand(task)
		}
		log.Println("Socket action requested:", data.Action)

	}
}

func HandleRequests(d bool) {
	debug = d
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	config = LoadConfiguration(usr.HomeDir + "/.captaincore/data/config.json")
	// Summarize only. The config holds the API tokens, and stdout lands in the
	// service journal.
	fmt.Printf("Loaded config: %d token(s), %d server(s), host %s (%s)\n", len(config.Tokens), len(config.Servers), config.Host, config.SSLMode)
	database_file := usr.HomeDir + "/.captaincore/data/sql.db"
	db, err = gorm.Open(sqlite.Open(database_file), &gorm.Config{})
	//db, err = gorm.Open("sqlite3", database_file)
	if err != nil {
		panic("failed to connect database" + database_file)
	}

	initialMigration()

	var httpSrv *http.Server

	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/task/{id}/stream", checkSecurity(streamTask)).Methods("GET")
	router.HandleFunc("/task/{id}", checkSecurity(viewTask)).Methods("GET")
	router.HandleFunc("/task/{id}", checkSecurity(updateTask)).Methods("PUT")
	router.HandleFunc("/task/{id}", checkSecurity(deleteTask)).Methods("DELETE")
	router.HandleFunc("/tasks", checkSecurity(newTask)).Methods("POST")
	router.HandleFunc("/tasks", checkSecurity(allTasks)).Methods("GET")
	router.HandleFunc("/tasks/{page}", checkSecurity(allTasks)).Methods("GET")
	router.HandleFunc("/run", checkSecurity(newRun)).Methods("POST")
	router.HandleFunc("/run/stream", checkSecurity(newRunStream)).Methods("POST")
	router.HandleFunc("/run/background", checkSecurity(newBackground)).Methods("POST")
	router.HandleFunc("/progress", checkSecurity(handleProgress)).Methods("GET")
	router.HandleFunc("/progress/{pid}", checkSecurity(handleProgressDetail)).Methods("GET")
	router.HandleFunc("/progress/{pid}", checkSecurity(handleProgressKill)).Methods("DELETE")
	router.HandleFunc("/assets/{name}", assetHandler)
	router.HandleFunc("/ws", wsHandler)
	router.HandleFunc("/", handleIndex)

	// NOTE: ReadTimeout/WriteTimeout are intentionally NOT set. They apply to the
	// whole request/response — including hijacked websocket connections (/ws) and
	// the long-lived HTTP streaming endpoints (/run/stream, /task/{id}/stream).
	// A 60s WriteTimeout silently cut those off mid-stream (e.g. a sync-data with
	// a long, output-silent SSH phase). ReadHeaderTimeout still guards against
	// slow-header (slowloris) attacks, and IdleTimeout bounds idle keep-alive
	// between requests without affecting active streams.
	// Bind address. Defaults to ":8000" (all interfaces) for backwards
	// compatibility, but can be pinned to loopback behind a TLS-terminating
	// reverse proxy via CAPTAINCORE_SERVER_BIND=127.0.0.1:8000.
	bindAddr := ":8000"
	if env := os.Getenv("CAPTAINCORE_SERVER_BIND"); env != "" {
		bindAddr = env
	}
	httpSrv = &http.Server{
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		Handler:           router,
		Addr:              bindAddr,
	}
	fmt.Println("Starting server http://" + bindAddr)
	log.Fatal(httpSrv.ListenAndServe())
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, fmt.Sprintf(htmlIndexTemplate, version.Version))
}

type progressMeta struct {
	Command   string `json:"command"`
	Total     int    `json:"total"`
	PID       int    `json:"pid"`
	StartedAt int64  `json:"started_at"`
	CaptainID string `json:"captain_id"`
	Parallel  int    `json:"parallel"`
	Target    string `json:"target"`
	Args      string `json:"args"`
}

type progressOutput struct {
	Command        string  `json:"command"`
	Completed      int     `json:"completed"`
	Failed         int     `json:"failed"`
	Total          int     `json:"total"`
	Percent        float64 `json:"percent"`
	PID            int     `json:"pid"`
	Running        bool    `json:"running"`
	StartedAt      int64   `json:"started_at"`
	ElapsedSeconds int64   `json:"elapsed_seconds"`
	Parallel       int     `json:"parallel"`
	ETA            string  `json:"eta"`
	Elapsed        string  `json:"elapsed"`
	Target         string  `json:"target"`
	Args           string  `json:"args"`
}

func handleProgress(w http.ResponseWriter, r *http.Request) {
	captainID := fetchCaptainID(r.Header.Get("token"), r)

	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	progressDir := home + "/.captaincore/data/progress"
	entries, err := filepath.Glob(progressDir + "/*.json")
	if err != nil || len(entries) == 0 {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "[]")
		return
	}

	var results []progressOutput
	now := time.Now().Unix()

	for _, metaPath := range entries {
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var meta progressMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		// Only surface progress runs belonging to the calling tenant.
		if meta.CaptainID != captainID {
			continue
		}

		logPath := strings.TrimSuffix(metaPath, ".json") + ".log"
		completed, failed := progressCountLogLines(logPath)
		running := syscall.Kill(meta.PID, 0) == nil && meta.PID > 0
		elapsed := now - meta.StartedAt

		var pct float64
		if meta.Total > 0 {
			pct = float64(completed) / float64(meta.Total) * 100
			pct = float64(int(pct*10)) / 10
		}

		eta := ""
		if completed > 0 && running && completed < meta.Total {
			remaining := meta.Total - completed
			secsPerItem := float64(elapsed) / float64(completed)
			etaSecs := int64(float64(remaining) * secsPerItem)
			eta = progressFormatElapsed(etaSecs)
		}

		results = append(results, progressOutput{
			Command:        meta.Command,
			Completed:      completed,
			Failed:         failed,
			Total:          meta.Total,
			Percent:        pct,
			PID:            meta.PID,
			Running:        running,
			StartedAt:      meta.StartedAt,
			ElapsedSeconds: elapsed,
			Parallel:       meta.Parallel,
			ETA:            eta,
			Elapsed:        progressFormatElapsed(elapsed),
			Target:         meta.Target,
			Args:           meta.Args,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

type progressLogEntry struct {
	Site      string `json:"site"`
	ExitCode  int    `json:"exit_code"`
	Timestamp int64  `json:"timestamp"`
}

type progressDetailOutput struct {
	progressOutput
	Completed_Sites []progressLogEntry `json:"completed_sites"`
	Pending_Sites   []string           `json:"pending_sites"`
}

func handleProgressDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	// Validate pid is a positive integer before using it in a file path —
	// prevents path traversal via the {pid} route segment.
	pid, perr := strconv.Atoi(vars["pid"])
	if perr != nil || pid <= 0 {
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}
	captainID := fetchCaptainID(r.Header.Get("token"), r)

	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	progressDir := home + "/.captaincore/data/progress"
	metaPath := filepath.Join(progressDir, strconv.Itoa(pid)+".json")

	data, err := os.ReadFile(metaPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var meta progressMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		http.Error(w, "invalid meta", http.StatusInternalServerError)
		return
	}

	// Only the owning tenant may read a progress run's detail.
	if meta.CaptainID != captainID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	logPath := strings.TrimSuffix(metaPath, ".json") + ".log"
	completed, failed := progressCountLogLines(logPath)
	running := syscall.Kill(meta.PID, 0) == nil && meta.PID > 0
	now := time.Now().Unix()
	elapsed := now - meta.StartedAt

	var pct float64
	if meta.Total > 0 {
		pct = float64(completed) / float64(meta.Total) * 100
		pct = float64(int(pct*10)) / 10
	}

	eta := ""
	if completed > 0 && running && completed < meta.Total {
		remaining := meta.Total - completed
		secsPerItem := float64(elapsed) / float64(completed)
		etaSecs := int64(float64(remaining) * secsPerItem)
		eta = progressFormatElapsed(etaSecs)
	}

	// Parse log entries
	var logEntries []progressLogEntry
	completedSet := make(map[string]bool)
	if f, err := os.Open(logPath); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) == 0 {
				continue
			}
			entry := progressLogEntry{Site: parts[0]}
			if len(parts) >= 2 {
				entry.ExitCode, _ = strconv.Atoi(parts[1])
			}
			if len(parts) >= 3 {
				entry.Timestamp, _ = strconv.ParseInt(parts[2], 10, 64)
			}
			logEntries = append(logEntries, entry)
			completedSet[parts[0]] = true
		}
	}

	// Determine pending sites from target list
	var pending []string
	if meta.Target != "" {
		targets := strings.Fields(meta.Target)
		for _, t := range targets {
			if !completedSet[t] {
				pending = append(pending, t)
			}
		}
	}

	result := progressDetailOutput{
		progressOutput: progressOutput{
			Command:        meta.Command,
			Completed:      completed,
			Failed:         failed,
			Total:          meta.Total,
			Percent:        pct,
			PID:            meta.PID,
			Running:        running,
			StartedAt:      meta.StartedAt,
			ElapsedSeconds: elapsed,
			Parallel:       meta.Parallel,
			ETA:            eta,
			Elapsed:        progressFormatElapsed(elapsed),
			Target:         meta.Target,
			Args:           meta.Args,
		},
		Completed_Sites: logEntries,
		Pending_Sites:   pending,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleProgressKill(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pid, err := strconv.Atoi(vars["pid"])
	if err != nil || pid <= 0 {
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}

	captainID := fetchCaptainID(r.Header.Get("token"), r)

	home, _ := os.UserHomeDir()
	progressDir := home + "/.captaincore/data/progress"
	metaPath := filepath.Join(progressDir, strconv.Itoa(pid)+".json")
	logPath := filepath.Join(progressDir, strconv.Itoa(pid)+".log")
	lockPath := filepath.Join(progressDir, strconv.Itoa(pid)+".lock")

	// Require a progress record owned by the caller before signalling any
	// process. This prevents killing arbitrary host PIDs or another tenant's job.
	metaData, metaErr := os.ReadFile(metaPath)
	if metaErr != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var meta progressMeta
	if json.Unmarshal(metaData, &meta) != nil {
		http.Error(w, "invalid meta", http.StatusInternalServerError)
		return
	}
	if meta.CaptainID != captainID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	running := syscall.Kill(pid, 0) == nil
	status := "cleaned"

	if running {
		// Kill the process group (negative PID) to stop xargs children too
		err = syscall.Kill(-pid, syscall.SIGTERM)
		if err != nil {
			// Fallback: kill just the process
			err = syscall.Kill(pid, syscall.SIGTERM)
		}
		if err != nil {
			http.Error(w, "failed to kill process", http.StatusInternalServerError)
			return
		}
		status = "killed"
	}

	// Clean up progress files
	os.Remove(metaPath)
	os.Remove(logPath)
	os.Remove(lockPath)

	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, fmt.Sprintf(`{"status":"%s"}`, status))
}

func progressCountLogLines(path string) (completed, failed int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		completed++
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] != "0" {
			failed++
		}
	}
	return completed, failed
}

func progressFormatElapsed(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func initialMigration() {
	// Migrate the schema
	db.AutoMigrate(&Task{})
}

func isJSON(str string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(str), &js) == nil
}

func killCommand(t Task) {
	syscall.Kill(-t.ProcessID, syscall.SIGKILL)
	syscall.Kill(t.ProcessID, syscall.SIGKILL)
	log.Println("Process killed ", strconv.Itoa(t.ProcessID))
}

// parseCommandString tokenizes a legacy command string into an executable head
// and its arguments using the historical regex, preserving the --command=/--name=
// double-quote stripping quirk. Used only for clients that still send a "command"
// string; the new "args" array protocol skips this entirely.
func parseCommandString(cmd string) (string, []string) {
	// See https://regexr.com/4154h for custom regex to parse commands
	// Inspired by https://gist.github.com/danesparza/a651ac923d6313b9d1b7563c9245743b
	pattern := `(--[^\s]+="[^"]+")|"([^"]+)"|'([^']+)'|([^\s]+)`
	parts := regexp.MustCompile(pattern).FindAllString(cmd, -1)
	if len(parts) == 0 {
		return "", nil
	}
	head := parts[0]
	arguments := parts[1:]
	for i, v := range arguments {
		if strings.HasPrefix(v, "--command=") || strings.HasPrefix(v, "--name=") {
			arguments[i] = strings.Replace(v, "\"", "", -1)
		}
	}
	return head, arguments
}

// writePayloadFile stores a large payload blob to disk, keyed by task token.
func writePayloadFile(token, data string) {
	usr, err := user.Current()
	if err != nil {
		log.Println("payload: cannot resolve user:", err)
		return
	}
	payloadDir := usr.HomeDir + "/.captaincore/data/payload"
	os.MkdirAll(payloadDir, 0700)
	// Payloads can contain secrets — write 0600, not world-readable.
	if err := os.WriteFile(payloadDir+"/"+token+".txt", []byte(data), 0600); err != nil {
		log.Println("payload: write failed:", err)
	}
}

// removePayloadFile deletes a consumed payload blob. Best-effort; a missing file
// (e.g. tasks that never had a payload) is fine.
func removePayloadFile(token string) {
	if token == "" {
		return
	}
	usr, err := user.Current()
	if err != nil {
		return
	}
	os.Remove(usr.HomeDir + "/.captaincore/data/payload/" + token + ".txt")
}

// prepareArgvTask bakes the new array protocol onto a task: writes any payload
// blob to disk (appending --payload=<token>), persists the argv as JSON, and
// sets a human-readable Command for display. No-op for legacy string requests.
func prepareArgvTask(t *Task) {
	if len(t.Args) == 0 {
		return
	}
	if t.Payload != "" {
		writePayloadFile(t.Token, t.Payload)
		t.Args = append(t.Args, "--payload="+t.Token)
		t.Payload = ""
	}
	if b, err := json.Marshal(t.Args); err == nil {
		t.ArgsJSON = string(b)
	}
	t.Command = "captaincore " + strings.Join(t.Args, " ")
}

// reservedTaskFlags are the global flags the server owns on a task's behalf. A
// caller must never supply its own copy: pflag keeps the LAST occurrence of a
// flag, so a trailing --captain-id would re-scope the run onto another tenant,
// --fleet fans it out across every tenant at once, and --config repoints the
// CLI at a different config file. They are dropped from caller argv before the
// server prepends the --captain-id its token resolved to.
var reservedTaskFlags = map[string]bool{
	"--captain-id": true,
	"--fleet":      true,
	"--config":     true,
}

// stripReservedFlags removes any reserved global flag (and its separate value,
// for the "--flag value" form) from a caller-supplied argument list.
func stripReservedFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name := arg
		hasInlineValue := false
		if idx := strings.Index(arg, "="); idx >= 0 {
			name = arg[:idx]
			hasInlineValue = true
		}
		if !reservedTaskFlags[name] {
			out = append(out, arg)
			continue
		}
		log.Printf("task: dropped reserved flag %s from caller arguments", name)
		// "--captain-id 7" carries its value in the next element.
		if !hasInlineValue && name != "--fleet" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
		}
	}
	return out
}

// buildExec resolves the executable + arguments for a task. New protocol tasks
// (ArgsJSON set, or Args still in memory) run captaincore with the argv verbatim;
// legacy tasks fall back to tokenizing the command string. In both cases the
// tenant's --captain-id is prepended by the server and any caller-supplied copy
// of a reserved global flag is discarded first.
func buildExec(t Task, captainID string) (string, []string) {
	var argv []string
	if t.ArgsJSON != "" {
		json.Unmarshal([]byte(t.ArgsJSON), &argv)
	} else if len(t.Args) > 0 {
		argv = t.Args
	}
	if len(argv) > 0 {
		return "captaincore", append([]string{"--captain-id=" + captainID}, stripReservedFlags(argv)...)
	}
	head, args := parseCommandString("captaincore " + t.Command)
	if head == "" {
		return "", nil
	}
	return head, append([]string{"--captain-id=" + captainID}, stripReservedFlags(args)...)
}

// runStreamCommand executes a command and streams its binary output directly to the HTTP response.
func runStreamCommand(w http.ResponseWriter, head string, arguments []string, t Task) {
	log.Printf("Running stream command for Task %d: %s", t.ID, t.Command)
	// if db != nil {
	// 	db.Model(&task).Update("Status", "Running")
	// }

	command := exec.Command(head, arguments...)
	command.Stdout = w
	command.Stderr = os.Stderr // Pipe stderr to the server log for debugging

	w.Header().Set("Content-Type", "application/octet-stream")

	err := command.Run()

	if err != nil {
		log.Printf("Error running stream command for Task %d: %v", t.ID, err)
		// if db != nil {
		// 	db.Model(&task).Update("Status", "Failed")
		// }
	} else {
		log.Printf("Stream command for Task %d completed successfully.", t.ID)
		// if db != nil {
		// 	db.Model(&task).Update("Status", "Completed")
		// }
	}
}

func runCommand(head string, arguments []string, t Task) string {

	// Find current connection write data
	var client Client
	for _, c := range clients {
		if c.Token == t.Token {
			client = c
			break
		}
	}

	// Format the command
	command := exec.Command(head, arguments...)

	// Sanity check -- capture stdout and stderr:
	stdout, _ := command.StdoutPipe() // Standard out: out.String()
	stderr, _ := command.StderrPipe() // Standard errors: stderr.String()

	// Setup process group
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Run the command
	err := command.Start()
	if err != nil {
		// Don't crash the daemon if a command fails to start; record it and bail.
		log.Printf("cmd.Start() failed with '%s'\n", err)
		t.Status = "Failed"
		t.Response = "Failed to start command: " + err.Error()
		db.Save(&t)
		return t.Response
	}

	// Grab proccess id
	t.ProcessID = command.Process.Pid
	db.Save(&t)

	if debug == true {
		fmt.Println("Starting command process ID " + strconv.Itoa(command.Process.Pid))
	}

	s := bufio.NewScanner(io.MultiReader(stdout, stderr))
	// Default Scanner caps a line at 64 KB; past that Scan() errors out, the
	// pipe stops draining, and the child blocks on write forever — the request
	// then hangs until the caller's HTTP timeout. Allow long lines instead.
	s.Buffer(make([]byte, 64*1024), 8*1024*1024)
	lines := []string{}
	for s.Scan() {
		// Write data to websocket if found
		if client.Token == t.Token {
			if debug == true {
				log.Println("Writting to socket:", client)
			}
			client.conn.WriteMessage(1, s.Bytes())
		}
		// Write data for final output
		lines = append(lines, s.Text())
	}

	err = command.Wait()
	if err != nil && client.conn != nil {
		client.conn.WriteMessage(1, []byte("Error: "+err.Error()))
		client.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		client.conn.Close()
	}

	// Clean up websocket if found
	if client.Token == t.Token {
		client.conn.WriteMessage(1, []byte("Finished."))
		client.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		client.conn.Close()
	}

	t.Status = "Completed"
	t.Response = strings.Join(lines, "\n")

	// If origin set then make request to mark that completed
	if t.Origin != "" {
		var origin Origin
		json.Unmarshal([]byte(t.Origin), &origin)

		fmt.Println("Updating origin server " + origin.Server + " Job ID " + origin.ID)

		// Build URL
		url := "https://" + origin.Server + "/task/" + origin.ID

		client := &http.Client{}
		client.Timeout = time.Second * 15

		req, err := http.NewRequest(http.MethodPut, url, nil)
		if err != nil {
			// Origin callback is best-effort — never crash the daemon over it.
			log.Printf("origin http.NewRequest() failed with '%s'\n", err)
		} else {
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
			req.Header.Add("token", origin.Token)

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("origin client.Do() failed with '%s'\n", err)
			} else {
				resp.Body.Close()
			}
		}
	}

	// Clean up task progress file if it exists
	if t.ProcessID > 0 {
		home, _ := os.UserHomeDir()
		progressPath := filepath.Join(home, ".captaincore", "data", "task-progress", strconv.Itoa(t.ProcessID)+".json")
		os.Remove(progressPath)
	}

	// Remove the consumed payload blob so secrets don't linger on disk.
	removePayloadFile(t.Token)

	db.Save(&t)
	output := strings.Join(lines, "\n")

	if debug == true {
		log.Println("scanner output:", lines)
		for _, v := range command.Args {
			fmt.Println(v)
		}
	}
	return output

}

func checkSecurity(next httpHandlerFunc) httpHandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		header := req.Header.Get("token")
		unauthorized := true
		for _, v := range config.Tokens {
			// Constant-time compare to avoid leaking token bytes via timing.
			if subtle.ConstantTimeCompare([]byte(v.Token), []byte(header)) == 1 {
				unauthorized = false
			}
		}
		if unauthorized {
			res.WriteHeader(http.StatusUnauthorized)
			res.Write([]byte("401 - Unauthorized"))
			return
		}
		next(res, req)
	}
}
