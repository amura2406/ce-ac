package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gomodule/redigo/redis"
	"github.com/rs/cors"
)

var (
	redisPool *redis.Pool
	authToken string
)

func main() {
	authToken = mustGetenv("PUBSUB_VERIFICATION_TOKEN")

	redisHost := mustGetenv("REDISHOST")
	redisPort := mustGetenv("REDISPORT")
	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

	const maxConnections = 10
	redisPool = redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", redisAddr)
	}, maxConnections)

	mux := http.NewServeMux()
	mux.HandleFunc("/", healthCheckHandler)
	mux.HandleFunc("/pubsub/push", pushHandler)
	mux.HandleFunc("/search", autocompleteHandler)

	handler := cors.AllowAll().Handler(mux)

	log.Fatal(http.ListenAndServe(":8080", handler))
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

	conn := redisPool.Get()
	defer conn.Close()

	results, err := redis.Strings(conn.Do("ZRANGEBYLEX", queryStr, "-", "+", "LIMIT", "0", "10"))
	if err != nil {
		http.Error(w, "Error connecting to redis", http.StatusInternalServerError)
		return
	}

	respJson := []ProductDoc{}
	for _, term := range results {
		p := &ProductDoc{
			Name: term,
		}
		respJson = append(respJson, *p)
	}
	json.NewEncoder(w).Encode(respJson)
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

	conn := redisPool.Get()
	defer conn.Close()

	term := msg.Message.Data.Name
	substr := term[:2]
	termLen := len(term)
	_, err := redis.Int(conn.Do("ZADD", substr, termLen, term))
	if err != nil {
		http.Error(w, "Error connecting to redis", http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, "OK")
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "OK")
}
