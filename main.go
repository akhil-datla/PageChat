package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
	"github.com/ReneKroon/ttlcache/v2"
	"github.com/gorilla/websocket"
	"github.com/pterm/pterm"
	"github.com/TwiN/go-away"
)

var clients = make(map[string][]*websocket.Conn) // connected clients
var broadcast = make(chan Message)               // broadcast channel
// Configure the upgrader
var upgrader = websocket.Upgrader{}

var cache ttlcache.SimpleCache = ttlcache.NewCache()

// Define our message object
type Message struct {
	Website   string          `json:"website"`
	Username  string          `json:"username"`
	Message   string          `json:"message"`
	Websocket *websocket.Conn `json:"-"`
}

func main() {
	banner()
	// Read port flag
	portPtr := flag.Int("port", 8080, "port to listen on")
	flag.Parse()

	// Set up the cache with a TTL of 24 hours
	cache.SetTTL(time.Duration(24 * time.Hour))

	// Configure websocket route
	http.HandleFunc("/ws", handleConnections)

	// Get past messages for a website
	http.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		website := r.URL.Query().Get("website")
		if website == "" {
			pterm.Error.Println("No website specified")
			return
		}
		messages, _ := cache.Get(website)

		if messages == nil {
			messages = make([]Message, 0)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(messages)
	})

	// Start listening for incoming chat messages
	go handleMessages()
	// Start the server on localhost port and log any errors
	pterm.Info.Printf("http server started on port %d\n", *portPtr)
	err := http.ListenAndServe(fmt.Sprintf(":%d", *portPtr), nil)
	if err != nil {
		pterm.Error.Println("ListenAndServe: ", err)
		os.Exit(1)
	}

	// Capture ctrl+c
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	cache.Close()
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	// Upgrade initial GET request to a websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	// Make sure we close the connection when the function returns
	defer ws.Close()

	for {
		var msg Message
		// Read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&msg)

		// Register our new client
		var clientFound bool
		for _, client := range clients[msg.Website] {
			if client == ws {
				clientFound = true
				break
			}
		}

		if !clientFound {
			clients[msg.Website] = append(clients[msg.Website], ws)
			pterm.Info.Println("Client connected")
		}

		if err != nil {
			pterm.Error.Printf("error: %v", err)
			for i, client := range clients[msg.Website] {
				if client == ws {
					remove(clients[msg.Website], i)
				}
			}
			break
		}
		msg.Websocket = ws
		// Send the newly received message to the broadcast channel
		broadcast <- msg
	}

}

func handleMessages() {
	for {
		// Grab the next message from the broadcast channel
		msg := <-broadcast

		censoredMessage := goaway.Censor(msg.Message)

		tempMsgs, _ := cache.Get(msg.Website)
		if tempMsgs == nil {
			tempMsgs = make([]Message, 0)
		}
		tempMsgs = append(tempMsgs.([]Message), Message{msg.Website, msg.Username, censoredMessage, msg.Websocket})
		cache.Set(msg.Website, tempMsgs)

		clientsByWebsite := clients[msg.Website]
		for _, client := range clientsByWebsite {
			err := client.WriteJSON(Message{msg.Website, msg.Username, censoredMessage, msg.Websocket})
			if err != nil {
				pterm.Error.Printf("error: %v", err)
				client.Close()
				for i, client := range clients[msg.Website] {
					if client == msg.Websocket {
						remove(clients[msg.Website], i)
					}
				}
			}
		}
	}
}

func remove(s []*websocket.Conn, i int) []*websocket.Conn {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

//banner for the program
func banner() {
	pterm.DefaultCenter.Print(pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgCyan)).WithMargin(10).Sprint("PageChat"))
	pterm.Info.Println("(c)2022 by Akhil Datla")

}
