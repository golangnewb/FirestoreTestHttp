package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"text/template"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

var (
	projectID     = ""
	indexTemplate = template.Must(template.ParseFiles("index.html"))
	editTemplate  = template.Must(template.ParseFiles("edit.html"))
)

func main() {
	if projectID == "" {
		log.Fatalf("set Firestore projectID project ID via GCLOUD_PROJECT env variable.")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/capitals", capitalCitiesOnly)
	mux.HandleFunc("/create", createCityHandler)
	mux.HandleFunc("/edit/", editCityHandler)
	http.ListenAndServe(":"+port, mux)

}

// Console Document view:
// https://console.cloud.google.com/firestore/data?_ga=2.155000894.-700545183.1539639625&project=firestoredemo-257414&folder=&organizationId=
func indexHandler(w http.ResponseWriter, r *http.Request) {

	ctx := context.Background()

	client, _ := firestore.NewClient(ctx, projectID)

	query := client.Collection("cities").Documents(ctx)
	defer query.Stop()
	cities := make([]*CityWithRef, 0)
	for {
		doc, err := query.Next()

		if err == iterator.Done {
			break
		}

		c := doc.Data()
		city := extractCityData(c)
		cityDocumentRef := doc.Ref

		cityWithRef := CityWithRef{
			City:   &city,
			CityID: cityDocumentRef.ID,
		}
		cities = append(cities, &cityWithRef)
	}

	err := indexTemplate.Execute(w, cities)
	if err != nil {
		fmt.Printf("\n **** indexTemplate.Execute err is: %s", err)
	}
}

func editCityHandler(w http.ResponseWriter, r *http.Request) {

	ctx := context.Background()
	client, _ := firestore.NewClient(ctx, projectID)

	// City DocRef.ID is passed in URL
	cityID := r.URL.Path[len("/edit/"):]

	dsnap, err := client.Collection("cities").Doc(cityID).Get(ctx)

	if err != nil {
		fmt.Printf("\n **** editCItyHandler1 dsnap Get err is: %s \n", err)
	}
	cityData := dsnap.Data()
	cityDocRef := dsnap.Ref

	// is there a way I can get the Document with the shortUrl?
	switch r.Method {
	case "GET":
		city := City{
			Name:       cityData["name"].(string),
			Country:    cityData["country"].(string),
			Population: cityData["population"].(int64),
		}

		state, ok := cityData["state"].(string)
		if ok {
			city.State = state
		}

		_, ok = cityData["capital"]
		if ok {
			city.Capital = cityData["capital"].(bool)
		}

		cityWithRef := CityWithRef{
			City:   &city,
			CityID: cityDocRef.ID,
		}
		// Document.Ref is not a string
		//  *cloud.google.com/go/firestore.DocumentRef

		// send city data to edit.html page
		err := editTemplate.Execute(w, cityWithRef)
		if err != nil {
			fmt.Printf("\n **** editTemplate.Execute err is: %s", err)
		}
	case "POST":
		// get the DocumentRef from Firestore
		err := r.ParseForm()

		// In case of any error, we respond with an error to the user
		if err != nil {
			fmt.Println(fmt.Errorf("Error Parsing Form in editCityHandler: %v \n", err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var capital bool
		if r.Form.Get("capital") != "" {
			capital, err = strconv.ParseBool(r.Form.Get("capital"))
		}
		var population int64
		if r.Form.Get("population") != "" {
			population, err = strconv.ParseInt(r.Form.Get("population"), 10, 64)
		}

		if err != nil {
			fmt.Println(fmt.Errorf("error parsing form editCityHandler: %v \n", err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		log.Printf("**** editCityHandler POST r.Form values before UpDate: %#v \n", r.Form)
		_, err = cityDocRef.Update(ctx, []firestore.Update{
			{Path: "population", Value: population},
			{Path: "name", Value: r.Form.Get("name")},
			{Path: "state", Value: r.Form.Get("state")},
			{Path: "country", Value: r.Form.Get("country")},
			{Path: "capital", Value: capital},
		})

		// // How do I get the DocRef for the Edit?
		if err != nil {
			// Handle any errors in an appropriate way, such as returning them.
			log.Printf("editCityHandler failed to POST updated city to Collection %s", err)
		}
		// this should 303redirect to index page
		http.Redirect(w, r, "/", http.StatusSeeOther)
		//303redirect isfound under a different URI and SHOULD be retrieved using a GET method on that resource. This method exists primarily to allow the output of a POST-activated script to redirect the user agent to a selected resource. The new URI is not a substitute reference for the originally requested resource. The 303 response MUST NOT be cached, but the response to the second (redirected) request might be cacheable.
	}

}

// this might be replacable with DataTo()
func extractCityData(c map[string]interface{}) (city City) {
	city = City{
		Name:       c["name"].(string),
		Country:    c["country"].(string),
		Population: c["population"].(int64),
	}
	// This is my ugly solution to dealing with nil value from Firestore
	_, ok := c["capital"]
	if ok {
		city.Capital = c["capital"].(bool)
	}
	state, ok := c["state"].(string)
	if ok {
		city.State = state
	}
	return city
}

func capitalCitiesOnly(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	client, _ := firestore.NewClient(ctx, projectID)

	query := client.Collection("cities").Where("capital", "==", true).Documents(ctx)
	defer query.Stop()
	cities := make([]*City, 0)
	for {
		doc, err := query.Next()
		if err == iterator.Done {
			break
		}

		c := doc.Data()
		city := extractCityData(c)
		cities = append(cities, &city)
	}
	err := indexTemplate.Execute(w, cities)
	if err != nil {
		fmt.Printf("\n **** indexTemplate.Execute err is: %s", err)
	}
}
func createCityHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	client, _ := firestore.NewClient(ctx, projectID)
	// create new instance of City
	city := City{}

	// We send all our data as HTML form data
	// the `ParseForm` method of the request, parses the
	// form values
	err := r.ParseForm()

	// In case of any error, we respond with an error to the user
	if err != nil {
		fmt.Println(fmt.Errorf("Error Parsing Form in createCityHandler: %v", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Get the information about the City from the Form info
	city.Name = r.Form.Get("name")
	city.State = r.Form.Get("state")
	city.Country = r.Form.Get("country")
	city.Population, _ = strconv.ParseInt(r.Form.Get("population"), 10, 64)
	city.Capital, _ = strconv.ParseBool(r.Form.Get("capital"))

	_, _, err = client.Collection("cities").Add(ctx, city)
	if err != nil {
		// Handle any errors in an appropriate way, such as returning them.
		log.Printf("CreateCity failed to POST city to Collection %s", err)
	}
	// this should 303redirect to index page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

type CityWithRef struct {
	*City
	CityID string
}
type City struct {
	Name       string `firestore:"name,omitempty"`
	State      string `firestore:"state,omitempty"`
	Country    string `firestore:"country,omitempty"`
	Capital    bool   `firestore:"capital,omitempty"`
	Population int64  `firestore:"population,omitempty"`
}
