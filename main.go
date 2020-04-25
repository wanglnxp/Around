package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	elastic "github.com/olivere/elastic/v7"
	"github.com/pborman/uuid"
)

const (
	// INDEX is elasticsearch index name
	INDEX = "around"
	// TYPE : elasticsearch type (diff in v7)
	TYPE = "post"
	// DISTANCE is default search range
	DISTANCE = "200km"
	// Needs to update
	//PROJECT_ID = "around-xxx"
	//BT_INSTANCE = "around-post"
	// Needs to update this URL if you deploy it to cloud.
	// ESURL : elasticseach url
	ESURL = "http://35.223.59.191:9200"
)

// Location is location json struct
type Location struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Post is one post message json struct
type Post struct {
	// `json:"user"` is for the json parsing of this User field. Otherwise, by default it's 'User'.
	User     string   `json:"user"`
	Message  string   `json:"message"`
	Location Location `json:"location"`
}

// Save a post to ElasticSearch
func saveToES(p *Post, id string) {
	// Create a client
	esClient, err := elastic.NewClient(elastic.SetURL(ESURL), elastic.SetSniff(false))
	if err != nil {
		panic(err)
	}

	// Save it to index
	_, err = esClient.Index().
		Index(INDEX).
		Id(id).
		BodyJson(p).
		Refresh("false").
		Do(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Printf("Post is saved to Index: %s\n", p.Message)
}

func handlerPost(w http.ResponseWriter, r *http.Request) {
	// Parse from body of request to get a json object.
	fmt.Println("Received one post request")
	decoder := json.NewDecoder(r.Body)
	var p Post
	if err := decoder.Decode(&p); err != nil {
		panic(err)
	}
	id := uuid.New()
	// Save to ES.
	saveToES(&p, id)
}

func handlerSearch(w http.ResponseWriter, r *http.Request) {
	// Parse from body of request to get a json object.
	fmt.Println("Received one request for search")
	lat, _ := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, _ := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	// range is optional
	ran := DISTANCE
	if val := r.URL.Query().Get("range"); val != "" {
		ran = val + "km"
	}

	fmt.Printf("Search received: %f %f %s\n", lat, lon, ran)

	// Create a client
	client, err := elastic.NewClient(elastic.SetURL(ESURL), elastic.SetSniff(false))
	if err != nil {
		panic(err)
	}

	// Define geo distance query as specified in
	// https://www.elastic.co/guide/en/elasticsearch/reference/5.2/query-dsl-geo-distance-query.html
	q := elastic.NewGeoDistanceQuery("location")
	q = q.Distance(ran).Lat(lat).Lon(lon)

	// Some delay may range from seconds to minutes. So if you don't get enough results. Try it later.
	searchResult, err := client.Search().
		Index(INDEX).
		Query(q).
		Pretty(true).
		Do(context.Background())
	if err != nil {
		panic(err)
	}

	// searchResult is of type SearchResult and returns hits, suggestions,
	// and all kinds of other information from Elasticsearch.
	fmt.Printf("Query took %d milliseconds\n", searchResult.TookInMillis)
	// TotalHits is another convenience function that works even when something goes wrong.
	fmt.Printf("Found a total of %d post\n", searchResult.TotalHits())

	// Each is a convenience function that iterates over hits in a search result.
	// It makes sure you don't need to check for nil values in the response.
	// However, it ignores errors in serialization.
	var typ Post
	var ps []Post
	for _, item := range searchResult.Each(reflect.TypeOf(typ)) { // instance of
		p := item.(Post) // p = (Post) item
		fmt.Printf("Post by %s: %s at lat %v and lon %v\n", p.User, p.Message, p.Location.Lat, p.Location.Lon)
		// Perform filtering based on keywords such as web spam etc.
		ns := strings.Replace(p.Message, "shit", "***", -1)
		ns = strings.Replace(p.Message, "bitch", "***", -1)
		p.Message = ns
		ps = append(ps, p)
	}
	js, err := json.Marshal(ps)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(js)
}

func main() {
	// Create a client
	client, err := elastic.NewClient(elastic.SetURL(ESURL), elastic.SetSniff(false))
	if err != nil {
		panic(err)
	}

	// Use the IndexExists service to check if a specified index exists.
	exists, err := client.IndexExists(INDEX).Do(context.Background())
	if err != nil {
		panic(err)
	}
	if !exists {
		// Create a new index.
		mapping := `{
			"mappings":{
				"properties":{
					"location":{
						"type":"geo_point"
					}
				}
			}
		}`
		_, err := client.CreateIndex(INDEX).Body(mapping).Do(context.Background())
		if err != nil {
			panic(err)
		}
	}

	fmt.Println("started-service")
	http.HandleFunc("/post", handlerPost)
	http.HandleFunc("/search", handlerSearch)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
