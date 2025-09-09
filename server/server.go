package server

import (
	"bufio"
	"crypto/rand"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const letternumberBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var db *gorm.DB
var err error
var config Config
var debug bool

//go:embed logo*.png
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
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type httpHandlerFunc func(http.ResponseWriter, *http.Request)

const (
	htmlIndex = `<html><head><title>CaptainCore</title>
<link rel="icon" href="assets/logo-32x32.png" sizes="32x32" />
<link rel="icon" href="assets/logo-192x192.png" sizes="192x192" />
<link rel="apple-touch-icon" href="assets/logo-180x180.png" />
<meta name="msapplication-TileImage" content="assets/logo-270x270.png" />
<script type='text/javascript' src='https://buttons.github.io/buttons.js?ver=5.7.1' id='github-buttons-js'></script>
<style>
body { margin:0px;font-family:-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif, 'Apple Color Emoji', 'Segoe UI Emoji', 'Segoe UI Symbol'; text-align:center; background-color: #fff; }
h2 { color:#fff;padding:5%;background:#1565c0; }
h2 a { color: #fff; }
a { color:#1565c0; text-decoration:none; padding: 3px 0px; margin: 0em 1em 1em 1em; display: inline-block; border-bottom: 1px solid #1565c0; }
a:hover { opacity: 0.75 }
</style>
</head>
<body>
<h2><a href="https://captaincore.io"><img src="assets/logo-192x192.png" style="width:32px; filter: invert(100%) grayscale(100%) brightness(100); top: 7px; position: relative; margin-right: 4px;">CaptainCore</a><small style="font-size: .57em;">v0.13.0</small></h2>
<p><a target="_blank" href="https://docs.captaincore.io">Docs 📖</a><a target="_blank" href="https://captaincore.io/development-updates/">Development Updates 🔔</a></p>
<p><a class="github-button" href="https://github.com/sponsors/austinginder" data-icon="octicon-heart" data-size="large" aria-label="Sponsor @austinginder on GitHub">Sponsor via Github</a></p>
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
}

type Task struct {
	gorm.Model
	CaptainID int
	ProcessID int
	Command   string
	Status    string
	Response  string
	Origin    string
	Token     string
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
	for _, v := range config.Tokens {
		if v.Token == t {
			return v.CaptainID
		}
	}
	return "0"
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

	task.Status = "Started"
	task.CaptainID, err = strconv.Atoi(captainID)
	task.Token = randomToken

	db.Create(&task)

	// Starts running CaptainCore command
	response := runCommand("captaincore --captain-id="+captainID+" "+task.Command, task)
	fmt.Fprintf(w, response)
}

func newRunStream(w http.ResponseWriter, r *http.Request) {
	var task Task
	json.NewDecoder(r.Body).Decode(&task)
	token := r.Header.Get("token")
	randomToken := generateToken()
	captainID := fetchCaptainID(token, r)

	task.Status = "Started"
	task.CaptainID, err = strconv.Atoi(captainID)
	task.Token = randomToken

	db.Create(&task)

	// Starts running CaptainCore command
	runStreamCommand(w, "captaincore --captain-id="+captainID+" "+task.Command, task)
}

func newBackground(w http.ResponseWriter, r *http.Request) {
	var task Task
	json.NewDecoder(r.Body).Decode(&task)
	token := r.Header.Get("token")
	randomToken := generateToken()
	captainID := fetchCaptainID(token, r)

	task.Status = "Started"
	task.CaptainID, err = strconv.Atoi(captainID)
	task.Token = randomToken

	// If command contains payload="<payload>" then then write data to file and change to payload="true"
	pattern := `(--payload='.+')`
	payload := regexp.MustCompile(pattern).FindString(task.Command)

	if len(payload) >= 1 {
		log.Println("Payload found, writing to file.")
		task.Command = strings.Replace(task.Command, payload, task.Token, -1)

		pattern_data := `--payload='(.+)'`
		payload_data := regexp.MustCompile(pattern_data).FindStringSubmatch(payload)

		usr, err := user.Current()
		if err != nil {
			log.Fatal(err)
		}

		writeerr := WriteToFile(usr.HomeDir+"/.captaincore/data/payload/"+task.Token+".txt", payload_data[1])
		if writeerr != nil {
			log.Fatal(writeerr)
		}
	}

	db.Create(&task)

	// Starts running CaptainCore command
	go runCommand("captaincore --captain-id="+captainID+" "+task.Command, task)
	fmt.Fprintf(w, "Successfully Started Task "+task.Token)
}

func newTask(w http.ResponseWriter, r *http.Request) {
	var task Task
	json.NewDecoder(r.Body).Decode(&task)
	token := r.Header.Get("token")
	randomToken := generateToken()
	captainID := fetchCaptainID(token, r)
	task.Status = "Queued"
	task.CaptainID, err = strconv.Atoi(captainID)
	task.Token = randomToken

	// If command contains payload="<payload>" then then write data to file and change to payload="true"
	pattern := `(--payload='.+')`
	payload := regexp.MustCompile(pattern).FindString(task.Command)

	if len(payload) >= 1 {
		log.Println("Payload found, writing to file.")
		task.Command = strings.Replace(task.Command, payload, task.Token, -1)

		pattern_data := `--payload='(.+)'`
		payload_data := regexp.MustCompile(pattern_data).FindStringSubmatch(payload)

		usr, err := user.Current()
		if err != nil {
			log.Fatal(err)
		}

		writeerr := WriteToFile(usr.HomeDir+"/.captaincore/data/payload/"+task.Token+".txt", payload_data[1])
		if writeerr != nil {
			log.Fatal(writeerr)
		}
	}

	db.Create(&task)
	taskID := strconv.FormatUint(uint64(task.ID), 10)
	response := "{ \"task_id\" : " + taskID + ", \"token\" : \"" + randomToken + "\" }"
	fmt.Fprintf(w, response)
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
	command := vars["command"]

	var tasks Task
	db.Where("command = ?", command).Find(&tasks)
	db.Delete(&tasks)

	fmt.Fprintf(w, "Successfully Deleted Task")
}

func viewTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	token := r.Header.Get("token")
	captainID := fetchCaptainID(token, r)

	var tasks Task
	db.Where("id = ?", id).Where("captain_id = ?", captainID).Find(&tasks)
	fmt.Println("{}", tasks)
	json.NewEncoder(w).Encode(tasks)
}

func updateTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	token := r.Header.Get("token")
	captainID := fetchCaptainID(token, r)

	var task Task
	db.Where("id = ?", id).Where("captain_id = ?", captainID).Find(&task)
	task.Status = "Completed"
	db.Save(&task)

	fmt.Fprintf(w, "Successfully Updated Task")
}

func logoHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	size, _ := vars["size"]
	template.ParseFS(staticFiles, "logo-"+size+".png")
	//http.ServeFile(w, r, "server/logo-"+size+".png")
	data, _ := staticFiles.ReadFile("logo-" + size + ".png")
	//w.writeHead(200, {"Content-Type": "image/png"});
	w.Header().Set("Content-Type", mime.TypeByExtension("png"))
	w.Write(data)
	//res.Header().Set("Content-Type", "text/html")
	//fmt.Fprint(data)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	// Make sure we close the connection when the function returns
	defer conn.Close()

	newClient := Client{Token: "", conn: conn, send: make(chan []byte, 256)}
	clients = append(clients, newClient)
	log.Println("Successfully established connection", clients)

	for {
		data := SocketRequest{}
		err := conn.ReadJSON(&data)
		if err != nil {
			fmt.Println("Error reading json.", err)
		}

		var task Task
		db.Where("token = ?", data.Token).Find(&task)

		// Refuse connection if token not valid
		if task.Token == "" {
			// Find current connection and remove from clients
			for i := 0; i < len(clients); i++ {
				if clients[i].conn == conn {
					log.Println("Removing client: ", clients[i])
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
			go runCommand("captaincore --captain-id="+captainID+" "+task.Command, task)
		}
		if data.Action == "listen" {
			captainID := strconv.Itoa(task.CaptainID)
			go runCommand("captaincore running listen --captain-id="+captainID, task)
		}
		if data.Action == "kill" {
			go killCommand(task)
		}
		log.Println("Socket data request:", data)
		log.Println("Executing command for client:", clients)

	}
}

func HandleRequests(d bool) {
	debug = d
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	config = LoadConfiguration(usr.HomeDir + "/.captaincore/data/config.json")
	fmt.Println(config)
	database_file := usr.HomeDir + "/.captaincore/data/sql.db"
	db, err = gorm.Open(sqlite.Open(database_file), &gorm.Config{})
	//db, err = gorm.Open("sqlite3", database_file)
	if err != nil {
		panic("failed to connect database" + database_file)
	}

	initialMigration()

	var httpSrv *http.Server

	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/task/{id}", checkSecurity(viewTask)).Methods("GET")
	router.HandleFunc("/task/{id}", checkSecurity(updateTask)).Methods("PUT")
	router.HandleFunc("/task/{id}", checkSecurity(deleteTask)).Methods("DELETE")
	router.HandleFunc("/tasks", checkSecurity(newTask)).Methods("POST")
	router.HandleFunc("/tasks", checkSecurity(allTasks)).Methods("GET")
	router.HandleFunc("/tasks/{page}", checkSecurity(allTasks)).Methods("GET")
	router.HandleFunc("/run", checkSecurity(newRun)).Methods("POST")
	router.HandleFunc("/run/stream", checkSecurity(newRunStream)).Methods("POST")
	router.HandleFunc("/run/background", checkSecurity(newBackground)).Methods("POST")
	router.HandleFunc("/assets/logo-{size}.png", logoHandler)
	router.HandleFunc("/ws", wsHandler)
	router.HandleFunc("/", handleIndex)

	httpSrv = &http.Server{
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      router,
		Addr:         ":8000",
	}
	fmt.Println("Starting server http://localhost:8000")
	log.Fatal(httpSrv.ListenAndServe())
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, htmlIndex)
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

// runStreamCommand executes a command and streams its binary output directly to the HTTP response.
func runStreamCommand(w http.ResponseWriter, cmd string, t Task) {
	// See https://regexr.com/4154h for custom regex to parse commands
	// Inspired by https://gist.github.com/danesparza/a651ac923d6313b9d1b7563c9245743b
	pattern := `(--[^\s]+="[^"]+")|"([^"]+)"|'([^']+)'|([^\s]+)`
	parts := regexp.MustCompile(pattern).FindAllString(cmd, -1)

	// The first part is the command, the rest are the args:
	head := parts[0]
	arguments := parts[1:len(parts)]

	// Loop through arguments and remove quotes from ---command="" due to bug
	for i, v := range arguments {
		if strings.HasPrefix(v, "--command=") {
			newArgument := strings.Replace(v, "\"", "", 1)
			newArgument = strings.Replace(newArgument, "\"", "", -1)
			arguments[i] = newArgument
		}
		if strings.HasPrefix(v, "--name=") {
			newArgument := strings.Replace(v, "\"", "", 1)
			newArgument = strings.Replace(newArgument, "\"", "", -1)
			arguments[i] = newArgument
		}
	}

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

func runCommand(cmd string, t Task) string {
	// See https://regexr.com/4154h for custom regex to parse commands
	// Inspired by https://gist.github.com/danesparza/a651ac923d6313b9d1b7563c9245743b
	pattern := `(--[^\s]+="[^"]+")|"([^"]+)"|'([^']+)'|([^\s]+)`
	parts := regexp.MustCompile(pattern).FindAllString(cmd, -1)

	// The first part is the command, the rest are the args:
	head := parts[0]
	//dirname, err := os.UserHomeDir()
	//path := dirname + "/.captaincore/app/"
	//head = "/bin/bash -c " + head
	arguments := parts[1:len(parts)]

	// log.Println("Hunting for socket with token ", t.Token)

	// Find current connection write data
	var client Client
	for _, c := range clients {
		log.Println("Client:", c.Token)
		if c.Token == t.Token {
			client = c
			break
		}
	}

	// Loop through arguments and remove quotes from ---command="" due to bug
	for i, v := range arguments {
		if strings.HasPrefix(v, "--command=") {
			newArgument := strings.Replace(v, "\"", "", 1)
			newArgument = strings.Replace(newArgument, "\"", "", -1)
			arguments[i] = newArgument
		}
		if strings.HasPrefix(v, "--name=") {
			newArgument := strings.Replace(v, "\"", "", 1)
			newArgument = strings.Replace(newArgument, "\"", "", -1)
			arguments[i] = newArgument
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

	// Grab proccess id
	t.ProcessID = command.Process.Pid
	db.Save(&t)

	if debug == true {
		fmt.Println("Starting command process ID " + strconv.Itoa(command.Process.Pid))
	}
	if err != nil {
		log.Fatalf("cmd.Start() failed with '%s'\n", err)
	}

	s := bufio.NewScanner(io.MultiReader(stdout, stderr))
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
			log.Fatalf("http.NewRequest() failed with '%s'\n", err)
		}

		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Add("token", origin.Token)

		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("client.Do() failed with '%s'\n", err)
		}

		defer resp.Body.Close()
		if err != nil {
			log.Fatalf("ioutil.ReadAll() failed with '%s'\n", err)
		}
	}

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
			if v.Token == header {
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
