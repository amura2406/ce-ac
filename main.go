package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"

	"cloud.google.com/go/pubsub"

	"google.golang.org/appengine"
	"google.golang.org/appengine/search"

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
		Data       Product
		ID         string `json:"message_id"`
	}
	Subscription string
}

type Product struct {
	ID    int64  `json:"sku"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

type ProductDoc struct {
	Name  string
	Image string
}

func autocompleteHandler(w http.ResponseWriter, r *http.Request) {
	queryStr := r.URL.Query().Get("q")

	ctx := appengine.NewContext(r)

	index, err := search.Open(searchIndex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := bytes.NewBufferString("")
	searchOpts := &search.SearchOptions{
		Limit: 10,
	}
	for t := index.Search(ctx, fmt.Sprintf("Name: %s", queryStr), searchOpts); ; {
		var doc ProductDoc
		id, err := t.Next(&doc)
		if err == search.Done {
			break
		}
		if err != nil {
			fmt.Fprintf(w, "Search error: %v\n", err)
			break
		}
		fmt.Fprintf(result, "%s -> %#v\n", id, doc)
	}
	fmt.Fprint(w, result.String())
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

	ctx := appengine.NewContext(r)

	index, err := search.Open(searchIndex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	p := msg.Message.Data
	_, err = index.Put(ctx, string(p.ID), &ProductDoc{
		Name:  p.Name,
		Image: p.Image,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, "OK")
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	messagesMu.Lock()
	defer messagesMu.Unlock()

	if err := tmpl.Execute(w, messages); err != nil {
		log.Printf("Could not execute template: %v", err)
	}
}

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
