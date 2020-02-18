package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

func TestCities(t *testing.T) {
	// setup code
	_ = addCitiesFirestore()
	var bostonDocumentID string
	// Go through subtests
	t.Run("Test POST should add New City", func(t *testing.T) {
		// Create Post Request with form data
		form := newCreateCityForm()

		req, err := http.NewRequest("POST", "", bytes.NewBufferString(form.Encode()))

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Content-Length", strconv.Itoa(len(form.Encode())))
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()

		hf := http.HandlerFunc(createCityHandler)

		hf.ServeHTTP(rr, req)
		// ends Create POST Request with form
		if status := rr.Code; status != http.StatusSeeOther {
			t.Errorf("createCityHandler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
		// get the New Record from the Firestore
		ctx := context.Background()
		client, _ := firestore.NewClient(ctx, projectID)
		query := client.Collection("cities").Where("name", "==", "Boston").Documents(ctx)
		defer query.Stop()
		cities := make([]*City, 0)
		for {
			doc, err := query.Next()

			if err == iterator.Done {
				break
			}
			c := doc.Data()
			bostonDocumentID = doc.Ref.ID
			city := extractCityData(c)
			cities = append(cities, &city)
		}

		expected := &City{
			Name:       "Boston",
			Country:    "USA",
			Population: 685000,
			Capital:    true,
		}
		// Remember when comparing structs to Compare *pointers which will be the data fields, otherwise &addresses will be compared
		if *cities[0] != *expected {
			t.Errorf("Why is ( cities[0] != expected )Boston was not added to Firestore: \n got %v  \n want %v", cities[0], expected)
		}
		got := cities[0]

		if got == expected {
			t.Errorf("got == expected; Boston was added to Firestore: got %v want %v", got, expected)
		}
	})
	t.Run("Capitals Only query", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/capitals", nil)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(indexHandler)
		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		ctx := context.Background()

		client, _ := firestore.NewClient(ctx, projectID)
		query := client.Collection("cities").Where("capital", "==", true).Documents(ctx)
		defer query.Stop()
		var capitalsCount int
		for {
			_, err := query.Next()

			if err == iterator.Done {
				break
			}
			capitalsCount++

		}
		expected := 2
		if capitalsCount != expected {
			t.Errorf("capitalsOnly query: got %v want %v", capitalsCount, expected)
		}
	})
	t.Run("should UPDATE city population", func(t *testing.T) {
		// get the DocumentKey for Boston
		ctx := context.Background()
		client, _ := firestore.NewClient(ctx, projectID)

		form := url.Values{}
		form.Set("name", "Boston")
		form.Set("state", "MA") // new data
		form.Set("country", "USA")
		form.Set("capital", "true")
		form.Set("population", "696969") // changed

		// This should be sent form data to URL
		req, err := http.NewRequest("POST", "/edit/"+bostonDocumentID, bytes.NewBufferString(form.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Content-Length", strconv.Itoa(len(form.Encode())))
		if err != nil {
			t.Errorf("request for /edit FAILED: %s", err)
		}

		rr := httptest.NewRecorder()

		hf := http.HandlerFunc(editCityHandler)

		hf.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusSeeOther {
			t.Errorf("editCityHandler returned wrong status code: got %v want %v", status, http.StatusSeeOther)
		}
		// After the updateCityHandler is completed I should
		// get updated Boston from the Firestore to compare results
		dsnapUpdated, err := client.Collection("cities").Doc(bostonDocumentID).Get(ctx)
		if err != nil {
			t.Errorf("could not find Boston for UpdateCityPopulation test: %s", err)
		}

		var cityDataUpdated City
		if err := dsnapUpdated.DataTo(&cityDataUpdated); err != nil {
			t.Errorf("DataTo(&cityDataUpdated) failed %s", err)
		}

		expected := &City{
			Name:       "Boston",
			State:      "MA",
			Country:    "USA",
			Population: 696969,
			Capital:    true,
		}

		if &cityDataUpdated == expected {
			t.Errorf("Edited value of Boston got: %v want: %v", &cityDataUpdated, expected)
		}
	})

	DeleteEntireCollection()
}

// clean up the Firestore
func DeleteEntireCollection() {
	// query for all records
	ctx := context.Background()
	client, _ := firestore.NewClient(ctx, projectID)
	// can I query for IDs only?
	query := client.Collection("cities").Documents(ctx)
	defer query.Stop()
	// it seems (q query) SelectPaths() will return only document IDs
	// iterate through the results and delete them.
	var citiesDeleted int
	for {
		doc, err := query.Next()
		if err == iterator.Done {
			break
		}

		_, err = client.Collection("cities").Doc(doc.Ref.ID).Delete(ctx)

		if err != nil {
			log.Printf("error FAIL deleting document from Firestore : %s", err)
		}
		citiesDeleted++
	}
	log.Printf("Preparing to DELETE %v city docs", citiesDeleted)
}

func newCreateCityForm() *url.Values {
	form := url.Values{}
	form.Set("name", "Boston")
	form.Set("country", "USA")
	form.Set("capital", "true") // how should checkbox be passed in Form?
	form.Set("population", "685000")
	return &form
}

//func addCitiesFirestore(ctx context.Context, client *firestore.Client) error {
func addCitiesFirestore() error {
	ctx := context.Background()
	client, _ := firestore.NewClient(ctx, projectID)
	// [START fs_query_create_examples]
	cities := []struct {
		id string
		c  City
	}{
		{id: "SF", c: City{Name: "San Francisco", State: "CA", Country: "USA", Capital: false, Population: 860000}},
		{id: "LA", c: City{Name: "Los Angeles", State: "CA", Country: "USA", Capital: false, Population: 3900000}},
		{id: "DC", c: City{Name: "Washington D.C.", Country: "USA", Capital: false, Population: 680000}},
		{id: "TOK", c: City{Name: "Tokyo", Country: "Japan", Capital: true, Population: 9000000}},
	}
	for _, c := range cities {
		if _, err := client.Collection("cities").Doc(c.id).Set(ctx, c.c); err != nil {
			return err
		}
	}
	return nil
}

// How do I get TestDeleteEntireColleciton to run at the end of the test?
// it could be a button on the index.html page

// How should Users be structured in a NoSQL database?
// Name as DocName?
// What data should be a subcollection vs Structs?
