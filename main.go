// Copyright 2015 Google Inc. All rights reserved.
// Use of this source code is governed by the Apache 2.0
// license that can be found in the LICENSE file.

// Sample pubsub demonstrates use of the cloud.google.com/go/pubsub package from App Engine flexible environment.
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"

	"cloud.google.com/go/pubsub"

	"google.golang.org/appengine"

	"golang.org/x/net/context"
)

var (
	topic *pubsub.Topic

	// Messages received by this instance.
	messagesMu sync.Mutex
	messages   []string

	authToken   string
	searchIndex string
)

const maxMessages = 10

func main() {
	ctx := context.Background()

	client, err := pubsub.NewClient(ctx, mustGetenv("GOOGLE_CLOUD_PROJECT"))
	if err != nil {
		log.Fatal(err)
	}

	authToken = mustGetenv("PUBSUB_VERIFICATION_TOKEN")
	searchIndex = mustGetenv("SEARCH_INDEX")

	// Create topic if it doesn't exist.
	topicName := mustGetenv("PUBSUB_TOPIC")
	topic, _ = client.CreateTopic(ctx, topicName)
	// Get handle for topic in case it already existed.
	if topic == nil {
		topic = client.Topic(topicName)
	}

	http.HandleFunc("/", listHandler)
	http.HandleFunc("/pubsub/push", pushHandler)
	http.HandleFunc("/search", autocompleteHandler)

	appengine.Main()
}

func mustGetenv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s environment variable not set.", k)
	}
	return v
}

type pushRequest struct {
	Message struct {
		Attributes map[string]string
		Data       []byte
		ID         string `json:"message_id"`
	}
	Subscription string
}

type Product struct {
	ID   string
	Name string
}

func autocompleteHandler(w http.ResponseWriter, r *http.Request) {
	queryStr := r.URL.Query().Get("q")

	// ctx := appengine.NewContext(r)

	// index, err := search.Open(searchIndex)
	// if err != nil {
	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
	// 	return
	// }

	fmt.Fprint(w, queryStr)
}

func pushHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if authToken != token {
		http.Error(w, fmt.Sprintln("Invalid Token"), 403)
		return
	}

	msg := &pushRequest{}
	if err := json.NewDecoder(r.Body).Decode(msg); err != nil {
		http.Error(w, fmt.Sprintf("Could not decode body: %v", err), http.StatusBadRequest)
		return
	}

	messagesMu.Lock()
	defer messagesMu.Unlock()
	// Limit to ten.
	messages = append(messages, string(msg.Message.Data))
	if len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}

	// ctx := appengine.NewContext(r)
	// index, err := search.Open(searchIndex)
	// if err != nil {
	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
	// 	return
	// }

	// _, err = index.Put(ctx, id, user)
	// if err != nil {
	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
	// 	return
	// }

	fmt.Fprint(w, "OK")
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	messagesMu.Lock()
	defer messagesMu.Unlock()

	if err := tmpl.Execute(w, messages); err != nil {
		log.Printf("Could not execute template: %v", err)
	}
}

// func publishHandler(w http.ResponseWriter, r *http.Request) {
// 	ctx := context.Background()

// 	msg := &pubsub.Message{
// 		Data: []byte(r.FormValue("payload")),
// 	}

// 	if _, err := topic.Publish(ctx, msg).Get(ctx); err != nil {
// 		http.Error(w, fmt.Sprintf("Could not publish message: %v", err), 500)
// 		return
// 	}

// 	fmt.Fprint(w, "Message published.")
// }

var tmpl = template.Must(template.New("").Parse(`<!DOCTYPE html>
<html>
  <head>
    <title>Pub/Sub</title>
  </head>
  <body>
    <div>
      <p>Last ten messages received by this instance:</p>
      <ul>
      {{ range . }}
          <li>{{ . }}</li>
      {{ end }}
      </ul>
    </div>
    <!-- [START form] -->
    <form method="post" action="/pubsub/publish">
      <textarea name="payload" placeholder="Enter message here"></textarea>
      <input type="submit">
    </form>
    <!-- [END form] -->
    <p>Note: if the application is running across multiple instances, each
      instance will have its own list of messages.</p>
  </body>
</html>`))
