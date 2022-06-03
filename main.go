package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/rs/cors"
)

type Employee struct {
	Uniqid string //`json:"UUID"`
	Empid  string //`json:"Employee ID"`
	Fname  string //`json:"First Name"`
	Lname  string //`json:"Last Name"`
}

type Task struct {
	Unitid     string //`json:"GUID"`
	Taskid     string //`json:"Task ID"`
	Assignedto string //`json:"Assigned To"`
	Title      string //`json:"Title"`
	Privacy    string //`json:"Privacy"`
}

type Whois struct {
	Uniqid     string //`json:"Task UUID"`
	Empid      string //`json:"Employee UUID"`
	Fname      string //`json:"Task ID"`
	Lname      string //`json:"Assigned To"`
	Unitid     string //`json:"Title"`
	Taskid     string //`json:"Privacy"`
	Assignedto string //`json:"Employee ID"`
	Title      string //`json:"First Name"`
	Privacy    string //`json:"Last Name"`
}

type Message struct {
	Table  string           `json:"table"`
	Id     int              `json:"id"`
	Action string           `json:"action"`
	Data   *json.RawMessage `json:"data"`
}

const (
	host     = "localhost"
	port     = 5432
	psqluser = "postgres"
	password = "postgres"
	dbname   = "workers"
)

var (
	psqlInfo = fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, psqluser, password, dbname)

	// https                 all the other normal URL stuff								elastic endpoint
	// transport-----v      user:pass    @ elastic URL                                 /index/_doc
	elasticIndex = "https://test:ryantest@cointest.es.us-central1.gcp.cloud.es.io:9243/newindex/_doc"
	//http://localhost:9200/{index_name}/_doc" /// NO TRAILING SLASHES!!!
	esURLasArray = []string{elasticIndex}
	myIndex      = "newindex"
	ctx          = context.Background()
	cfg          = elasticsearch.Config{
		//Addresses: esURLasArray,
		//Username:  "test",
		//Password:  "ryantest",
		CloudID: "cointest:dXMtY2VudHJhbDEuZ2NwLmNsb3VkLmVzLmlvJDcxY2I3N2ZkYTUwZDQxMjQ4YWQzNDlkMTFjMzAyZmEzJGYyNTQwMmJiNTdkMzQ0NTg5NmY2NmM0MTY3MDE2YTJj",
		APIKey:  "emR5ekI0RUJKUlZnRGc2QURHX286bFVYSnBoZXVUd3Fqel9pek5aQnplQQ==",
	}

	fieldName  = "title" // static test input for searchESAPI.go
	searchTerm = "scrum" // static test input
	query      = `{"query": {"term": {"` + fieldName + `" : "` + searchTerm + `"}}, "size": 10}`

	verbose = true
)

func OpenConnection() *sql.DB {

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	return db
}

func main() {
	es, _ := elasticsearch.NewClient(cfg)
	//check that it works
	log.Println(elasticsearch.Version)

	res, err := es.Info()
	if err != nil {
		log.Fatalf("Error opening Elastic Index: %s", err)
	}
	log.Println(res)
	//it prints the elastic cluster info to the console to prove it works.

	go psqlEventListener()
	handleRequests()

}

func handleRequests() {
	// Cross Origin Handler
	corsWrapper := cors.New(cors.Options{
		AllowedMethods: []string{"GET", "POST", "PUT"},
		AllowedHeaders: []string{"Content-Type", "Origin", "Accept", "*"},
	})

	apiListener := mux.NewRouter().StrictSlash(true)
	apiListener.HandleFunc("/", homePage)

	// Elasticsearch return api endpoints
	apiListener.HandleFunc("/search/tasks/name/{title}", returnSingleTask_esapi)
	apiListener.HandleFunc("/search/employees/uuid/{uniqid}", returnSingleEmpUUID_esapi)
	apiListener.HandleFunc("/search/whois/assigned/{taskid}", returnEmployeesByTask_esapi) // !!TODO!!  mux & display

	// Postgres return api endpoints
	apiListener.HandleFunc("/employees", returnAllEmployees_psql)
	apiListener.HandleFunc("/employees/uuid/{uniqid}", returnSingleByUUID_psql)
	apiListener.HandleFunc("/employees/empid/{empid}", returnSingleEmployee_psql)
	apiListener.HandleFunc("/tasks", returnPublicTasks_psql)
	apiListener.HandleFunc("/tasks/name/{title}", returnSingleTask_psql)
	apiListener.HandleFunc("/whois", returnAllPairedTasks_psql)
	apiListener.HandleFunc("/whois/{taskid}", returnPairedTask_byID_psql)
	apiListener.HandleFunc("/other", othervars)

	//-------->>>>>  		other routes go here!			<<<<<<-------//

	log.Fatal(http.ListenAndServe(":8080", corsWrapper.Handler(apiListener)))
	return
	//-----------------------------------------------------------//
	//-----------------------------------------------------------//
	//		apiListener.HandleFunc("/employees/newEmployee", createNewEmployee).Methods("POST")
	//	!!TODO!	TechDebt: The test did not require a POST api function
	//		apiListener.HandleFunc("/employees/admin/UPD_ROW/{emid}", updateEmployee).Methods("update")
	//	!!TODO!	TechDebt: The test did not require a UPDATE api function
	//		apiListener.HandleFunc("/employees/admin/DEL_ROW/{uuid}", deleteEmployee).Methods("DELETE")
	//	!!TODO!	TechDebt: The test did not require a DELETE api function
	//-----------------------------------------------------------//
}
func psqlEventListener() {
	_, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}

	reportProblems := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf(err.Error())
		}
	}

	listener := pq.NewListener(psqlInfo, 10*time.Second, time.Minute, reportProblems)
	err = listener.Listen("events")
	if err != nil {
		panic(err)
	}
	log.Printf("PSQL is now listening...")
	for {
		elasticNotify(listener)
	}
}
func elasticNotify(l *pq.Listener) {
	for {
		select {
		case n := <-l.Notify:
			if verbose {
				log.Printf("Incoming Data...")
			}
			var prettyJSON bytes.Buffer
			err := json.Indent(&prettyJSON, []byte(n.Extra), "", "\t")
			if err != nil {
				log.Printf("Error in JSON output: ", err)
				return
			}
			if verbose {
				log.Printf("Raw JSON Output: ")
				log.Printf(string(prettyJSON.Bytes()))
			}

			var message Message
			messBytes := []byte(string(prettyJSON.Bytes()))
			err2 := json.Unmarshal(messBytes, &message)
			if err2 != nil {
				log.Printf("There was a problem creating the JSON object: ", err2)
				return
			}

			fmt.Println("Before")
			s := []string{message.Table, strconv.Itoa(message.Id)}
			r := strings.Join(s, "_")
			fmt.Println(r)
			fmt.Println("After")
			elasticWrite(message)
			return
		case <-time.After(90 * time.Second):
			log.Printf("Listener Sleeping. (No Ingestion for 90 seconds): ")
			go func() {
				l.Ping()
			}()
			return
		}
	}
}
func elasticWrite(message Message) {

	table := message.Table
	if verbose {
		log.Printf("table : %s", table)
		fmt.Println(reflect.TypeOf(table))
	}

	action := message.Action
	if verbose {
		log.Printf("action : %s", action)
		fmt.Println(reflect.TypeOf(action))
	}

	idRef := message.Id
	if verbose {
		log.Printf("id : %s", strconv.Itoa(idRef))
		fmt.Println(reflect.TypeOf(action))
	}

	s := []string{message.Table, strconv.Itoa(message.Id)}
	tableAndId := strings.Join(s, "_")

	if action == "DELETE" {
		if verbose {
			log.Printf("DELETE %s", tableAndId)
		}
		if !elasticReq("DELETE", tableAndId, nil) {
			log.Printf("Failed to delete %s", tableAndId)
		}
	} else {
		if verbose {
			log.Printf("INDEX  %s", tableAndId)
		}
		r := bytes.NewReader([]byte(*message.Data))
		if !elasticReq("PUT", tableAndId, r) {
			log.Printf("Failed to index %s:\n%s", tableAndId, string(*message.Data))
		}
	}
}
func elasticReq(method, id string, reader io.Reader) bool {

	resp := httpReq(method, elasticIndex+"/"+id, reader)
	if resp == nil {
		return false
	}
	ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	return true
}
func httpReq(method, url string, reader io.Reader) *http.Response {
	req, err := http.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if err != nil {
		log.Fatal("HTTP request build failed: ", method, " ", url, ": ", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal("HTTP request failed: ", method, " ", url, ": ", err)
	}
	if isErrorHTTPCode(resp) {
		body, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		log.Print("HTTP error: ", resp.Status, ": ", string(body))
		return nil
	}
	return resp
}
func isErrorHTTPCode(resp *http.Response) bool {
	return resp.StatusCode < 200 || resp.StatusCode >= 300
}

func homePage(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Please specify an api endpoint")
	log.Println("Homepage requested! ")
}

func returnSingleTask_esapi(w http.ResponseWriter, hr *http.Request) {
	log.SetFlags(0)

	var (
		r  map[string]interface{}
		wg sync.WaitGroup
	)

	// Initialize a client with the default settings.
	//
	// An `ELASTICSEARCH_URL` environment variable will be used when exported.
	//
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("Error creating the client: %s", err)
	}
	vars := mux.Vars(hr)
	key := string("%%" + vars["title"] + "%%")
	fmt.Println("/employees was called! esAPI results incoming     returnSingleTask_esapi     key=: " + key)

	//rows, err := db.Query("SELECT * FROM tasks WHERE title like $1 and privacy = 0", key)
	for i, title := range []string{key} {
		wg.Add(1)

		go func(i int, title string) {
			defer wg.Done()

			// Build the request body.
			data, err := json.Marshal(struct{ Title string }{Title: title})
			if err != nil {
				log.Fatalf("Error marshaling document: %s", err)
			}

			// Set up the request object.
			req := esapi.IndexRequest{
				Index:      myIndex,
				DocumentID: strconv.Itoa(i + 1),
				Body:       bytes.NewReader(data),
				Refresh:    "true",
			}

			// Perform the request with the client.
			res, err := req.Do(context.Background(), es)
			if err != nil {
				log.Fatalf("Error getting response: %s", err)
			}
			defer res.Body.Close()

			if res.IsError() {
				log.Printf("[%s] Error indexing document ID=%d", res.Status(), i+1)
			} else {
				// Deserialize the response into a map.
				var r map[string]interface{}
				if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
					log.Printf("Error parsing the response body: %s", err)
				} else {
					// Print the response status and indexed document version.
					log.Printf("[%s] %s; version=%d", res.Status(), r["result"], int(r["_version"].(float64)))
				}
			}
		}(i, title)
	}
	wg.Wait()

	log.Println(strings.Repeat("-", 37))

	// 3. Search for the indexed documents
	//
	// Build the request body.
	var buf bytes.Buffer
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"title": key,
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		log.Fatalf("Error encoding query: %s", err)
	}

	// Perform the search request.
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex(myIndex),
		es.Search.WithBody(&buf),
		es.Search.WithTrackTotalHits(true),
		es.Search.WithPretty(),
	)
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		var e map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			log.Fatalf("Error parsing the response body: %s", err)
		} else {
			// Print the response status and error information.
			log.Fatalf("[%s] %s: %s",
				res.Status(),
				e["error"].(map[string]interface{})["type"],
				e["error"].(map[string]interface{})["reason"],
			)
		}
	}

	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		log.Fatalf("Error parsing the response body: %s", err)
	}
	// Print the response status, number of results, and request duration.
	log.Printf(
		"[%s] %d hits; took: %dms",
		res.Status(),
		int(r["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)),
		int(r["took"].(float64)),
	)
	// Print the ID and document source for each hit.
	for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {
		empBytes, _ := json.MarshalIndent(hit, "", "\t")

		w.Header().Set("Content-Type", "application/json")
		w.Write(empBytes)

		log.Printf(" * ID=%s, %s", hit.(map[string]interface{})["_id"], hit.(map[string]interface{})["_source"])
	}

	log.Println(strings.Repeat("=", 37))
} // ✔ //
func returnSingleEmpUUID_esapi(w http.ResponseWriter, hr *http.Request) {
	log.SetFlags(0)

	var (
		r  map[string]interface{}
		wg sync.WaitGroup
	)

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("Error creating the client: %s", err)
	}
	vars := mux.Vars(hr)
	key := string(vars["uniqid"])
	fmt.Println("/employees was called! esAPI results incoming     returnSingleEmpUUID_esapi     key=: " + key)

	for i, uniqid := range []string{key} {
		wg.Add(1)

		go func(i int, uniqid string) {
			defer wg.Done()

			// Build the request body.
			data, err := json.Marshal(struct{ Uniqid string }{Uniqid: uniqid})
			if err != nil {
				log.Fatalf("Error marshaling document: %s", err)
			}

			// Set up the request object.
			req := esapi.IndexRequest{
				Index:      myIndex,
				DocumentID: strconv.Itoa(i + 1),
				Body:       bytes.NewReader(data),
				Refresh:    "true",
			}
			w.Write(data)
			// Perform the request with the client.
			res, err := req.Do(context.Background(), es)
			if err != nil {
				log.Fatalf("Error getting response: %s", err)
			}
			defer res.Body.Close()

			if res.IsError() {
				log.Printf("[%s] Error indexing document ID=%d", res.Status(), i+1)
			} else {
				// Deserialize the response into a map.
				var r map[string]interface{}
				if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
					log.Printf("Error parsing the response body: %s", err)
				} else {
					// Print the response status and indexed document version.
					log.Printf("[%s] %s; version=%d", res.Status(), r["result"], int(r["_version"].(float64)))
				}
			}
		}(i, uniqid)
	}
	wg.Wait()

	log.Println(strings.Repeat("-", 37))

	// 3. Search for the indexed documents
	//
	// Build the request body.
	var buf bytes.Buffer
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"uniqid": key,
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		log.Fatalf("Error encoding query: %s", err)
	}

	// Perform the search request.
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex(myIndex),
		es.Search.WithBody(&buf),
		es.Search.WithTrackTotalHits(true),
		es.Search.WithPretty(),
	)
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		var e map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			log.Fatalf("Error parsing the response body: %s", err)
		} else {
			// Print the response status and error information.
			log.Fatalf("[%s] %s: %s",
				res.Status(),
				e["error"].(map[string]interface{})["type"],
				e["error"].(map[string]interface{})["reason"],
			)
		}
	}

	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		log.Fatalf("Error parsing the response body: %s", err)
	}
	// Print the response status, number of results, and request duration.
	log.Printf(
		"[%s] %d hits; took: %dms",
		res.Status(),
		int(r["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)),
		int(r["took"].(float64)),
	)
	// Print the ID and document source for each hit.
	for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {
		empBytes, _ := json.MarshalIndent(hit, "", "\t")

		w.Header().Set("Content-Type", "application/json")
		w.Write(empBytes)

		log.Printf(" * ID=%s, %s", hit.(map[string]interface{})["_id"], hit.(map[string]interface{})["_source"])
	}

	log.Println(strings.Repeat("=", 37))
} // ✔ //
func returnEmployeesByTask_esapi(w http.ResponseWriter, hr *http.Request) {
	log.SetFlags(0)
	var (
		r  map[string]interface{}
		wg sync.WaitGroup
	)

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("Error creating the client: %s", err)
	}

	vars := mux.Vars(hr)
	key := string(vars["taskid"])
	fmt.Println("/employees was called! esAPI results incoming     returnSingleTask_esapi     key=: " + key)

	//rows, err := db.Query("SELECT * FROM tasks WHERE title like $1 and privacy = 0", key)
	for i, taskid := range []string{key} {
		wg.Add(1)

		go func(i int, taskid string) {
			defer wg.Done()

			// Build the request body.
			data, err := json.Marshal(struct{ Taskid string }{Taskid: taskid})
			if err != nil {
				log.Fatalf("Error marshaling document: %s", err)
			}

			// Set up the request object.
			req := esapi.IndexRequest{
				Index:      myIndex,
				DocumentID: strconv.Itoa(i + 1),
				Body:       bytes.NewReader(data),
				Refresh:    "true",
			}

			// Perform the request with the client.
			res, err7 := req.Do(context.Background(), es)
			if err7 != nil {
				log.Fatalf("Error getting response: %s", err7)
			}
			defer res.Body.Close()

			if res.IsError() {
				log.Printf("[%s] Error indexing document ID=%d", res.Status(), i+1)
			} else {
				// Deserialize the response into a map.
				var r map[string]interface{}
				if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
					log.Printf("Error parsing the response body: %s", err)
				} else {
					// Print the response status and indexed document version.
					log.Printf("[%s] %s; version=%d", res.Status(), r["result"], int(r["_version"].(float64)))
				}
			}
		}(i, taskid)
	}
	wg.Wait()

	log.Println(strings.Repeat("-", 37))

	// 3. Search for the indexed documents
	//
	// Build the request body.
	var buf bytes.Buffer
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"taskid": key,
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		log.Fatalf("Error encoding query: %s", err)
	}

	// Perform the search request.
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex(myIndex),
		es.Search.WithBody(&buf),
		es.Search.WithTrackTotalHits(true),
		es.Search.WithPretty(),
	)
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		var e map[string]interface{}
		if err8 := json.NewDecoder(res.Body).Decode(&e); err8 != nil {
			log.Fatalf("Error parsing the response body: %s", err8)
		} else {
			// Print the response status and error information.
			log.Fatalf("[%s] %s: %s",
				res.Status(),
				e["error"].(map[string]interface{})["type"],
				e["error"].(map[string]interface{})["reason"],
			)
		}
	}

	if err4 := json.NewDecoder(res.Body).Decode(&r); err4 != nil {
		log.Fatalf("Error parsing the response body: %s", err4)
	}
	// Print the response status, number of results, and request duration.
	log.Printf(
		"[%s] %d hits; took: %dms",
		res.Status(),
		int(r["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)),
		int(r["took"].(float64)),
	)
	// Print the ID and document source for each hit.
	for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {
		empBytes, _ := json.MarshalIndent(hit, "", "\t")

		w.Header().Set("Content-Type", "application/json")
		w.Write(empBytes)

		log.Printf(" * ID=%s, %s", hit.(map[string]interface{})["_id"], hit.(map[string]interface{})["_source"])
	}

	log.Println(strings.Repeat("=", 37))
} // ✔ //

func nope_returnEmployeesByTask_esapi(w http.ResponseWriter, hr *http.Request) {
	// /search/whois/assigned/{taskid}"
	log.SetFlags(0)
	var (
		r  map[string]interface{}
		wg sync.WaitGroup
	)

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("Error creating the client: %s", err)
	}

	vars := mux.Vars(hr)
	taskid_key := string(vars["taskid"])
	fmt.Println("/employees was called! esAPI results incoming    returnEmpByTask_esapi     key=: " + taskid_key)
	for i, taskid := range []string{taskid_key} {
		go func(i int, taskid string) {
			defer wg.Done()

			// Build the request body.
			data, err := json.Marshal(struct{ Taskid string }{Taskid: taskid})
			if err != nil {
				log.Fatalf("Error marshaling document: %s", err)
			}

			// Set up the request object.
			req := esapi.IndexRequest{
				Index:      myIndex, // <- this var is declared globally at the top of this file
				DocumentID: strconv.Itoa(i + 1),
				Body:       bytes.NewReader(data),
				Refresh:    "true",
			}

			// Perform the request with the client.
			res, err := req.Do(context.Background(), es)
			if err != nil {
				log.Fatalf("Error getting response: %s", err)
			}
			defer res.Body.Close()

			if res.IsError() {
				log.Printf("[%s] Error indexing document ID=%d", res.Status(), i+1)
			} else {
				// Deserialize the response into a map.
				var r map[string]interface{}
				if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
					log.Printf("Error parsing the response body: %s", err)
				} else {
					// Print the response status and indexed document version.
					log.Printf("[%s] %s; version=%d", res.Status(), r["result"], int(r["_version"].(float64)))
				}
			}
		}(i, taskid)
	}

	wg.Wait()

	log.Println(strings.Repeat("-", 37))

	// 3. Search for the indexed documents
	//
	// Build the request body.
	var buf bytes.Buffer
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"taskid": taskid_key,
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		log.Fatalf("Error encoding query: %s", err)
	}

	// Perform the search request.
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex(myIndex),
		es.Search.WithBody(&buf),
		es.Search.WithTrackTotalHits(true),
		es.Search.WithPretty(),
	)
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		var e map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			log.Fatalf("Error parsing the response body: %s", err)
		} else {
			// Print the response status and error information.
			log.Fatalf("[%s] %s: %s",
				res.Status(),
				e["error"].(map[string]interface{})["type"],
				e["error"].(map[string]interface{})["reason"],
			)
		}
	}

	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		log.Fatalf("Error parsing the response body: %s", err)
	}
	// Print the response status, number of results, and request duration.
	log.Printf(
		"[%s] %d hits; took: %dms",
		res.Status(),
		int(r["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)),
		int(r["took"].(float64)),
	)
	// Print the ID and document source for each hit.
	for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {
		empBytes, _ := json.MarshalIndent(hit, "", "\t")

		w.Header().Set("Content-Type", "application/json")
		w.Write(empBytes)

		log.Printf(" * ID=%s, %s", hit.(map[string]interface{})["_id"], hit.(map[string]interface{})["_source"])
	}

	log.Println(strings.Repeat("=", 37))
}

func returnAllEmployees_psql(w http.ResponseWriter, r *http.Request) {
	log.Println("/employees was called! apiServer.go:19 returnAllEmployees")

	//we could put the esAPI stuff here and if esAPI errors, then use psql
	//but i wanted to split them out to separate functions for this demo

	db := OpenConnection()
	rows, err := db.Query("SELECT * FROM employees")
	if err != nil {
		log.Fatal(err)
	}

	var employees []Employee

	for rows.Next() {
		var employee Employee
		rows.Scan(&employee.Uniqid, &employee.Empid, &employee.Fname, &employee.Lname)
		employees = append(employees, employee)
	}
	empBytes, _ := json.MarshalIndent(employees, "", "\t")

	w.Header().Set("Content-Type", "application/json")
	w.Write(empBytes)

	defer rows.Close()
	defer db.Close()

}
func returnPublicTasks_psql(w http.ResponseWriter, r *http.Request) {
	fmt.Println("/employees was called! apiServer.go:19 returnAllEmployees")

	db := OpenConnection()
	rows, err := db.Query("SELECT * FROM tasks")
	if err != nil {
		log.Fatal(err)
	}

	var tasks []Task
	for rows.Next() {
		var task Task
		rows.Scan(&task.Unitid, &task.Taskid, &task.Assignedto, &task.Title, &task.Privacy)
		tasks = append(tasks, task)
	}
	empBytes, _ := json.MarshalIndent(tasks, "", "\t")

	w.Header().Set("Content-Type", "application/json")
	w.Write(empBytes)

	defer rows.Close()
	defer db.Close()

}
func returnSingleEmployee_psql(w http.ResponseWriter, r *http.Request) {

	//do esAPI stuff here
	//and if esAPI errors,
	//then use psql

	vars := mux.Vars(r)
	key := vars["empid"]
	fmt.Println("/employees was called! apiServer.go:48 returnSingleEmployee     key=: " + key)
	db := OpenConnection()

	rows, err := db.Query("SELECT * FROM employees WHERE empid = " + key)
	if err != nil {
		log.Fatal(err)
	}

	var employees []Employee

	for rows.Next() {
		var employee Employee
		rows.Scan(&employee.Uniqid, &employee.Empid, &employee.Fname, &employee.Lname)
		employees = append(employees, employee)
	}
	empBytes, _ := json.MarshalIndent(employees, "", "\t")

	w.Header().Set("Content-Type", "application/json")
	w.Write(empBytes)

	defer rows.Close()
	defer db.Close()

}
func returnSingleByUUID_psql(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := string(vars["uniqid"])
	fmt.Println("/employees was called! apiServer.go:113 returnSingleByUUID     key=: " + key)

	//do esAPI stuff here
	//and if esAPI errors,
	//then use psql

	db := OpenConnection()
	//var temp1 = stri
	rows := db.QueryRow("SELECT * FROM employees WHERE uniqid::text = $1", key)

	var employees []Employee

	var employee Employee
	rows.Scan(&employee.Uniqid, &employee.Empid, &employee.Fname, &employee.Lname)
	employees = append(employees, employee)

	empBytes, _ := json.MarshalIndent(employees, "", "\t")

	w.Header().Set("Content-Type", "application/json")
	w.Write(empBytes)

	defer db.Close()
}
func returnSingleTask_psql(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := string("%%" + vars["title"] + "%%")
	fmt.Println("/employees was called! apiServer.go:101 returnSingleByTitle     key=: " + key)

	//do esAPI stuff here
	//and if esAPI errors,
	//then use psql

	db := OpenConnection()
	rows, err := db.Query("SELECT * FROM tasks WHERE title like $1 and privacy = 0", key)
	if err != nil {
		log.Fatal(err)
	}
	var tasks []Task
	for rows.Next() {
		var task Task
		rows.Scan(&task.Unitid, &task.Taskid, &task.Assignedto, &task.Title, &task.Privacy)
		tasks = append(tasks, task)
	}
	empBytes, _ := json.MarshalIndent(tasks, "", "\t")

	w.Header().Set("Content-Type", "application/json")
	w.Write(empBytes)

	defer rows.Close()
	defer db.Close()

}
func returnAllPairedTasks_psql(w http.ResponseWriter, r *http.Request) {

	fmt.Println("/employees was called: returnAllPairedTasks")

	db := OpenConnection()

	rows, err := db.Query("SELECT * FROM tasks inner join employees on empid=any(assignedto) where privacy=0")

	if err != nil {
		log.Fatal(err)
	}
	var whom []Whois
	for rows.Next() {
		var whois Whois
		rows.Scan(&whois.Uniqid, &whois.Empid, &whois.Fname, &whois.Lname, &whois.Unitid, &whois.Taskid, &whois.Assignedto, &whois.Title, &whois.Privacy)
		whom = append(whom, whois)
	}

	empBytes, _ := json.MarshalIndent(whom, "", "\t")

	w.Header().Set("Content-Type", "application/json")
	w.Write(empBytes)

	defer rows.Close()
	defer db.Close()

}
func returnPairedTask_byID_psql(w http.ResponseWriter, r *http.Request) {

	fmt.Println("/employees was called! apiServer.go:152 returnPairedTask_byID")

	db := OpenConnection()

	rows, err := db.Query("SELECT * FROM employees inner join tasks on $1 = any(assignedto)")

	if err != nil {
		log.Fatal(err)
	}
	var whom []Whois
	for rows.Next() {
		var whois Whois
		rows.Scan(&whois.Uniqid, &whois.Empid, &whois.Fname, &whois.Lname, &whois.Unitid, &whois.Taskid, &whois.Assignedto, &whois.Title, &whois.Privacy)
		whom = append(whom, whois)
	}

	empBytes, _ := json.MarshalIndent(whom, "", "\t")

	w.Header().Set("Content-Type", "application/json")
	w.Write(empBytes)

	//defer rows.Close()
	defer db.Close()

}

func another_way_to_search_ES_API(f string, s string) {

	fieldName = f
	searchTerm = s
	query = `{"query": {"term": {"` + fieldName + `" : "` + searchTerm + `"}}, "size": 10}`
	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("Elasticsearch connection error:", err)
	}

	var mapResp map[string]interface{}
	var buf bytes.Buffer

	// Concatenate a string from query for reading
	var b strings.Builder
	b.WriteString(query)
	read := strings.NewReader(b.String())

	fmt.Println("read:", read)
	fmt.Println("read TYPE:", reflect.TypeOf(read))
	fmt.Println("JSON encoding:", json.NewEncoder(&buf).Encode(read))

	// Attempt to encode the JSON query and look for errors
	if err := json.NewEncoder(&buf).Encode(read); err != nil {
		log.Fatalf("Error encoding query: %s", err)

		// Query is a valid JSON object
	} else {
		fmt.Println("json.NewEncoder encoded query:", read, "\n")

		// Pass the JSON query to the Golang client's Search() method
		res, err := client.Search(
			client.Search.WithContext(ctx),
			client.Search.WithIndex(searchTerm),
			client.Search.WithBody(read),
			client.Search.WithTrackTotalHits(true),
			client.Search.WithPretty(),
		)

		// Check for any errors returned by API call to Elasticsearch
		if err != nil {
			log.Fatalf("Elastic API ERROR:", err)
			// If no errors are returned, parse esapi.Response object
		} else {
			fmt.Println("res TYPE:", reflect.TypeOf(res))

			// Close the result body when the function call is complete
			defer res.Body.Close()

			// Decode the JSON response and set a pointer
			if err := json.NewDecoder(res.Body).Decode(&mapResp); err != nil {
				log.Fatalf("Error parsing the response body: %s", err)

				// If no error, then convert response to a map[string]interface
			} else {

				fmt.Println("mapResp TYPE:", reflect.TypeOf(mapResp), "\n")

				// Iterate the document "hits" returned by API call

				for _, hit := range mapResp["hits"].(map[string]interface{})["hits"].([]interface{}) {

					// Parse the attributes/fields of the document
					doc := hit.(map[string]interface{})
					// The "_source" data is another map interface nested inside of doc
					source := doc["_source"]
					fmt.Println("doc _source:", reflect.TypeOf(source))

					// Get the document's _id and print it out along with _source data
					docID := doc["_id"]
					fmt.Println("docID:", docID)
					fmt.Println("_source:", source, "\n")
				} // end of response iteration
			}
		}
	}
}

func othervars(w http.ResponseWriter, r *http.Request) {
	employeeData := []byte(`
		[
{
employees
		  {
			"uniqid": "b98291a1-69e9-4030-9afd-fd23a4d93f0f",
			"empid": 1,
			"fname": "jane",
			"lname": "smith"
		  },
		  {
			"uniqid": "22f2ece1-bccc-47eb-b28c-9bc767f3dc89",
			"empid": 2,
			"fname": "billy",
			"lname": "jones"
		  },
		  {
			"uniqid": "87fab4fb-441f-4cff-9121-2e3222ea7d18",
			"empid": 3,
			"fname": "lee",
			"lname": "irving"
		  },
		  {
			"uniqid": "f056f333-7836-471f-8578-47b24fd8b911",
			"empid": 4,
			"fname": "sarah",
			"lname": "pilsner"
		  },
		  {
			"uniqid": "72064bd1-b5b5-4911-b177-42609de697f9",
			"empid": 5,
			"fname": "guy",
			"lname": "young"
		  },
		  {
			"uniqid": "2abe1d24-72ca-4f03-9f7b-7478d025f716",
			"empid": 6,
			"fname": "lady",
			"lname": "oldman"
		  }
}
		]`)
	tasksData := []byte(`
		[{tasks
		  {
			"unitid": "18f16020-0e13-4f16-b4a4-e8dd52237d51",
			"taskid": 1,
			"assignedto": [1, 2, 3, 4, 5, 6],
			"title": "scrum meeting",
			"privacy": 0
		  },
		  {
			"unitid": "ca8af363-cbd5-4416-87f1-d65da242f69d",
			"taskid": 2,
			"assignedto": [4, 5, 6],
			"title": "interview",
			"privacy": 0
		  },
		  {
			"unitid": "2e819dbc-c15d-4e4f-896f-c0a95bd19b42",
			"taskid": 3,
			"assignedto": [1, 2, 6],
			"title": "documentation",
			"privacy": 0
		  },
		  {
			"unitid": "a9b89431-d8d6-41d2-be87-76c00ffe8484",
			"taskid": 4,
			"assignedto": [1, 3],
			"title": "secret docker file generation",
			"privacy": 1
		  },
		  {
			"unitid": "0a284be9-4033-4c53-8d6f-2b50e2ee5342",
			"taskid": 5,
			"assignedto": [2],
			"title": "a/b testing",
			"privacy": 0
		  },
		  {
			"unitid": "082b65f8-336a-4aa8-9869-6b1885441e4f",
			"taskid": 6,
			"assignedto": [1, 2],
			"title": "secret scrum meeting",
			"privacy": 1
		  },
		  {
			"unitid": "e5c2b757-f4c3-435a-b387-a231b158466a",
			"taskid": 7,
			"assignedto": [6],
			"title": "push dev to prod",
			"privacy": 0
		  },
		  {
			"unitid": "551fac4c-088e-402e-b597-4c1ce7f67cfe",
			"taskid": 8,
			"assignedto": [4, 6],
			"title": "secret scrum meeting",
			"privacy": 1
		  },
		  {
			"unitid": "82ded647-75bf-4a39-b071-fd80c4c95fd8",
			"taskid": 9,
			"assignedto": [],
			"title": "vacation",
			"privacy": 0
		  }
		}]
		`)
	w.Write(employeeData)
	w.Write(tasksData)
}
