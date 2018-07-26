package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	"google.golang.org/appengine"
	"google.golang.org/appengine/search"
)

var (
	// Messages received by this instance.
	messagesMu sync.Mutex
	messages   []string

	authToken   string
	searchIndex string
)

const maxMessages = 10

func main() {
	authToken = mustGetenv("PUBSUB_VERIFICATION_TOKEN")
	searchIndex = mustGetenv("SEARCH_INDEX")

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
		Data       struct {
			ID    int64  `json:"sku"`
			Name  string `json:"name"`
			Image string `json:"image"`
		}
		ID string `json:"message_id"`
	}
	Subscription string
}

type ProductDoc struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

func autocompleteHandler(w http.ResponseWriter, r *http.Request) {
	queryStr := r.URL.Query().Get("q")

	ctx := appengine.NewContext(r)

	index, err := search.Open(searchIndex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := []ProductDoc{}
	searchOpts := &search.SearchOptions{
		Limit: 10,
	}
	for t := index.Search(ctx, fmt.Sprintf("Name = %s", queryStr), searchOpts); ; {
		var doc ProductDoc
		_, err := t.Next(&doc)
		if err == search.Done {
			break
		}
		if err != nil {
			fmt.Fprintf(w, "Search error: %v\n", err)
			break
		}
		result = append(result, doc)
	}

	json.NewEncoder(w).Encode(result)
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
	_, err = index.Put(ctx, strconv.FormatInt(p.ID, 10), &ProductDoc{
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
		<title>Autocomplete Demo</title>
		<!-- JS file -->
		<script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/1.11.3/jquery.min.js"></script>
		<script src="https://cdnjs.cloudflare.com/ajax/libs/easy-autocomplete/1.3.5/jquery.easy-autocomplete.min.js"></script> 

		<!-- CSS file -->
		<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/easy-autocomplete/1.3.5/easy-autocomplete.min.css"> 
		<style>
			.container {
				display: flex;
				justify-content: center;
				align-items: center;
				width: 100%;
				height: 100vh;
			}
		</style>
  </head>
  <body>
		<div class="container">
			<input id="basics" />
		</div>
		<script>

			$(document).ready(function() {
				var options = {
					url: function(phrase) {
						return "search?q=" + phrase;
					},
				
					getValue: "name"
				};

				$("#basics").easyAutocomplete(options);
			});

		</script>
  </body>
</html>`))
