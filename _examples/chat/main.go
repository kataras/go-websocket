package main

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/kataras/go-websocket"
)

type clientPage struct {
	Title string
	Host  string
}

var host = "localhost:8080"

func main() {

	// serve our javascript files
	staticHandler := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", staticHandler))

	// Start Websocket example code

	// init our websocket
	ws := websocket.New(websocket.Config{}) // with the default configuration
	// the path which the websocket client should listen/registered to,
	// see ./templates/client.html line: 21
	// http.HandleFunc("/my_endpoint", ws.Handler)
	// ws.OnConnection(handleWebsocketConnection) // register the connection handler, which will fire on each new connected websocket client.
	// OR if you don't want to use the `OnConnection`
	// and you want to handle the Upgradation from simple http to websocket protocol do that:
	http.HandleFunc("/my_endpoint", func(w http.ResponseWriter, r *http.Request) {
		c := ws.Upgrade(w, r)
		if err := c.Err(); err != nil {
			http.Error(w, fmt.Sprintf("websocket error: %v\n", err), http.StatusServiceUnavailable)
		}

		// handle the connection.
		handleWebsocketConnection(c)

		// start the ping and the messages reader, this is blocking the http handler from exiting too.
		c.Wait()
	})

	// serve our client-side source code go-websocket. See ./templates/client.html line: 19
	http.HandleFunc("/go-websocket.js", func(w http.ResponseWriter, r *http.Request) {
		w.Write(ws.ClientSource)
	})

	// End Websocket example code

	// parse our view (the template file)
	clientHTML, err := template.New("").ParseFiles("./templates/client.html")
	if err != nil {
		panic(err)
	}

	// serve our html page to /
	http.Handle("/", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/" || req.URL.Path == "" {
			err := clientHTML.ExecuteTemplate(res, "client.html", clientPage{"Client Page", host})
			if err != nil {
				res.Write([]byte(err.Error()))
			}
		}
	}))

	println("Server is up & running, open two browser tabs/or/and windows and navigate to http://" + host)
	http.ListenAndServe(host, nil)
}

var myChatRoom = "room1"

func handleWebsocketConnection(c websocket.Connection) {

	c.Join(myChatRoom)

	c.On("chat", func(message string) {
		// to all except this connection (broadcast) ->
		// c.To(websocket.NotMe).Emit("chat", "Message from: "+c.ID()+"-> "+message)
		// to all: c.To(websocket.All).
		// to client itself->
		//c.Emit("chat", "Message from myself: "+message)

		//send the message to the whole room,
		//all connections are inside this room will receive this message
		c.To(myChatRoom).Emit("chat", "From: "+c.ID()+": "+message)
	})

	c.OnDisconnect(func() {
		fmt.Printf("\nConnection with ID: %s has been disconnected!", c.ID())
	})
}
