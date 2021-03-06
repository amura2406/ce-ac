package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/rs/cors"
)

var (
	redisPool *redis.Pool
	authToken string
	Info      *log.Logger
	Error     *log.Logger
)

func main() {
	Info = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(os.Stderr, "ERRO: ", log.Ldate|log.Ltime|log.Lshortfile)

	httpPort := mustGetenv("HTTPPORT")
	authToken = mustGetenv("PUBSUB_VERIFICATION_TOKEN")

	redisHost := mustGetenv("REDISHOST")
	redisPort := mustGetenv("REDISPORT")
	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

	redisPool = newRedisPool(redisAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/", healthCheckHandler)
	mux.HandleFunc("/pubsub/push", pushHandler)
	mux.HandleFunc("/search", autocompleteHandler)

	handler := cors.AllowAll().Handler(mux)

	log.Fatal(http.ListenAndServe(fmt.Sprint(":", httpPort), handler))
}

func newRedisPool(addr string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     10,
		MaxActive:   100,
		IdleTimeout: 240 * time.Second,
		Dial:        func() (redis.Conn, error) { return redis.Dial("tcp", addr) },
	}
}

func mustGetenv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s environment variable not set.", k)
	}
	return v
}

type PubSubPushRequest struct {
	Message struct {
		Attributes map[string]string
		Data       string
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
	Name  string `json:"name"`
	Image string `json:"image"`
}

func autocompleteHandler(w http.ResponseWriter, r *http.Request) {
	queryStr := r.URL.Query().Get("q")

	now := time.Now()
	conn := redisPool.Get()
	if conn.Err() != nil {
		http.Error(w, conn.Err().Error(), http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()
	timeTrack(now, "get redis connection")

	now = time.Now()
	results, err := redis.Strings(conn.Do("ZRANGEBYLEX", strings.ToLower(queryStr), "-", "+", "LIMIT", "0", "10"))
	if err != nil {
		http.Error(w, "Error connecting to redis", http.StatusInternalServerError)
		return
	}
	timeTrack(now, "call ZRANGEBYLEX")

	now = time.Now()
	respJson := []ProductDoc{}
	for _, term := range results {
		p := &ProductDoc{
			Name: term,
		}
		respJson = append(respJson, *p)
	}
	json.NewEncoder(w).Encode(respJson)
	timeTrack(now, "construct JSON response")
}

func pushHandler(w http.ResponseWriter, r *http.Request) {
	reqDump, err := httputil.DumpRequest(r, true)
	if err != nil {
		Error.Println("Couldn't dump request")
		http.Error(w, fmt.Sprintln("Couldn't dump request"), 500)
		return
	}

	token := r.URL.Query().Get("token")
	if authToken != token {
		http.Error(w, fmt.Sprintln("Invalid Token"), 403)
		return
	}

	msg := &PubSubPushRequest{}
	if err = json.NewDecoder(r.Body).Decode(msg); err != nil {
		Error.Println("Invalid request payload:", string(reqDump))
		http.Error(w, fmt.Sprintf("Could not decode body: %v", err), http.StatusBadRequest)
		return
	}

	productJson, err := base64.StdEncoding.DecodeString(msg.Message.Data)
	if err != nil {
		Error.Println("Could not decode base64 data:", msg.Message.Data)
		http.Error(w, fmt.Sprintf("Could not decode base64 data (%v): %v", err, msg.Message.Data), http.StatusBadRequest)
		return
	}

	product := &Product{}
	if err = json.Unmarshal(productJson, product); err != nil {
		Error.Println("Invalid request payload:", string(productJson))
		http.Error(w, fmt.Sprintf("Could not decode message: %v", err), http.StatusBadRequest)
		return
	}

	err = storeTermToRedis(product.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "OK")
}

func storeTermToRedis(term string) error {
	lTerm := strings.ToLower(term)
	termLen := len(lTerm)
	conn := redisPool.Get()
	if conn.Err() != nil {
		return conn.Err()
	}
	defer conn.Close()

	if termLen > 1 {
		const minChars = 2
		const maxChars = 5
		loopCount := int(math.Min(float64(termLen-1), float64(maxChars)))
		for i := 0; i < loopCount; i++ {
			numChar := minChars + i
			substr := lTerm[:numChar]

			Info.Println("ADD to redis set [", substr, "] :", term, "(len: ", termLen, ")")
			_, err := redis.Int(conn.Do("ZADD", substr, termLen, term))
			if err != nil {
				Error.Println(err)
				return err
			}
			Info.Println("ADD into [", substr, "] success")
		}

		Info.Println("Successfully add [", term, "]")
	} else {
		Info.Println("Skipping [", term, "]")
	}

	return nil
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	Info.Printf("%s took %s", name, elapsed)
}
