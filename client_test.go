package main

import (
	"encoding/json"
	"encoding/xml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

type Row struct {
	ID        int    `xml:"id"`
	IsActive  bool   `xml:"isActive"`
	FirstName string `xml:"first_name"`
	LastName  string `xml:"last_name"`
	About     string `xml:"about"`
	Age       int    `xml:"age"`
	Gender    string `xml:"gender"`
}

type DataSet struct {
	Rows []Row `xml:"row"`
}

var dataset DataSet

func init() {
	data, err := os.ReadFile("dataset.xml")
	if err != nil {
		panic(err)
	}
	err = xml.Unmarshal(data, &dataset)
	if err != nil {
		panic(err)
	}
}

func ServerSearch(w http.ResponseWriter, r *http.Request) {
	query := r.FormValue("query")
	orderField := r.FormValue("order_field")
	if orderField == "" {
		orderField = "Name"
	}
	orderByStr := r.FormValue("order_by")
	limitStr := r.FormValue("limit")
	offsetStr := r.FormValue("offset")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "invalid limit"}`, http.StatusBadRequest)
		return
	}
	if limit <= 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "limit must be > 0"}`, http.StatusBadRequest)
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "invalid offset"}`, http.StatusBadRequest)
		return
	}
	if offset < 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "offset must be > 0"}`, http.StatusBadRequest)
		return
	}

	orderBy, err := strconv.Atoi(orderByStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "invalid order_by"}`, http.StatusBadRequest)
		return
	}

	validOrderFields := map[string]bool{"Id": true, "Age": true, "Name": true}
	if !validOrderFields[orderField] {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"OrderField `+orderField+` invalid"}`, http.StatusBadRequest)
		return
	}

	var users []User
	for _, row := range dataset.Rows {
		name := row.FirstName + " " + row.LastName
		if query == "" || strings.Contains(strings.ToLower(name), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(row.About), strings.ToLower(query)) {
			users = append(users, User{
				Id:     row.ID,
				Name:   name,
				Age:    row.Age,
				About:  row.About,
				Gender: row.Gender,
			})
		}
	}

	if orderBy != OrderByAsIs {
		switch orderField {
		case "Id":
			sort.Slice(users, func(i, j int) bool {
				if orderBy == OrderByDesc {
					return users[i].Id > users[j].Id
				}
				return users[i].Id < users[j].Id
			})
		case "Age":
			sort.Slice(users, func(i, j int) bool {
				if orderBy == OrderByDesc {
					return users[i].Age > users[j].Age
				}
				return users[i].Age < users[j].Age
			})
		case "Name":
			sort.Slice(users, func(i, j int) bool {
				if orderBy == OrderByDesc {
					return users[i].Name > users[j].Name
				}
				return users[i].Name < users[j].Name
			})
		}
	}

	if offset >= len(users) {
		users = []User{}
	} else {
		users = users[offset:]
	}

	if len(users) > limit {
		users = users[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func TestFindUsers(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(ServerSearch))
	defer ts.Close()
	sc := SearchClient{
		AccessToken: "test_token",
		URL:         ts.URL,
	}
	tests := []struct {
		name           string
		req            SearchRequest
		expectedErr    string
		expectedLength int
		expectedNext   bool
		validateFunc   func(*testing.T, []User)
	}{
		{
			name: "EmptyQuery_ReturnsAll",
			req: SearchRequest{
				Limit: 25,
			},
			expectedLength: 25,
			expectedNext:   true,
		},
		{
			name: "LimitClampedTo25",
			req: SearchRequest{
				Limit: 100,
			},
			expectedLength: 25,
			expectedNext:   true,
		},
		{
			name: "OffsetBeyondData_EmptyResult",
			req: SearchRequest{
				Limit:  10,
				Offset: 1000,
			},
			expectedLength: 0,
			expectedNext:   false,
		},
		{
			name: "QueryInName",
			req: SearchRequest{
				Query: "Boyd Wolf",
				Limit: 1,
			},
			expectedLength: 1,
			expectedNext:   false,
		},
		{
			name: "QueryInAbout",
			req: SearchRequest{
				Query: "Nulla cillum enim",
				Limit: 1,
			},
			expectedLength: 1,
			expectedNext:   false,
		},
		{
			name: "OrderByInvalidField",
			req: SearchRequest{
				OrderField: "InvalidField",
				OrderBy:    OrderByAsc,
				Limit:      1,
			},
			expectedErr: "OrderField InvalidField invalid",
		},
		{
			name: "SortingByNameAsc",
			req: SearchRequest{
				OrderField: "Name",
				OrderBy:    OrderByAsc,
				Limit:      10,
			},
			expectedLength: 10,
			expectedNext:   true,
			validateFunc: func(t *testing.T, users []User) {
				for i := 1; i < len(users); i++ {
					assert.True(t, users[i-1].Name <= users[i].Name,
						"Names should be in ascending order")
				}
			},
		},
		{
			name: "SortingByAgeDesc",
			req: SearchRequest{
				OrderField: "Age",
				OrderBy:    OrderByDesc,
				Limit:      10,
			},
			expectedLength: 10,
			expectedNext:   true,
			validateFunc: func(t *testing.T, users []User) {
				for i := 1; i < len(users); i++ {
					assert.True(t, users[i-1].Age >= users[i].Age,
						"Ages should be in descending order")
				}
			},
		},
		{
			name: "SortingByIdAsIs",
			req: SearchRequest{
				OrderField: "Id",
				OrderBy:    OrderByAsIs,
				Limit:      10,
			},
			expectedLength: 10,
			expectedNext:   true,
		},
		{
			name: "TimeoutError",
			req: SearchRequest{
				Limit: 25,
			},
			expectedErr: "timeout",
		},
		{
			name: "Unauthorized",
			req: SearchRequest{
				Limit: 25,
			},
			expectedErr: "Bad AccessToken",
		},
		{
			name: "Internal Server Error",
			req: SearchRequest{
				Limit: 25,
			},
			expectedErr: "SearchServer fatal error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectedErr == "timeout" || tt.expectedErr == "Bad AccessToken" ||
				tt.expectedErr == "SearchServer fatal error" {
				ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if tt.expectedErr == "timeout" {
						time.Sleep(2 * time.Second)
						return
					}
					if tt.expectedErr == "Bad AccessToken" {
						http.Error(w, "unauthorized", http.StatusUnauthorized)
						return
					}
					if tt.expectedErr == "SearchServer fatal error" {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}
				}))
				defer ts2.Close()
				sc.URL = ts2.URL
				sc.AccessToken = "wrong_token"
			} else {
				sc.AccessToken = "test_token"
			}

			res, err := sc.FindUsers(tt.req)

			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				return
			}

			require.NoError(t, err)
			assert.Len(t, res.Users, tt.expectedLength)

			if tt.validateFunc != nil {
				tt.validateFunc(t, res.Users)
			}
		})
	}
}

func TestFindUsers_EdgeCases(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(ServerSearch))
	defer ts.Close()
	sc := SearchClient{AccessToken: "test_token", URL: ts.URL}
	total := len(dataset.Rows)

	cases := []struct {
		name        string
		req         SearchRequest
		expectErr   string
		expectLen   int
		skipIfEmpty bool
		nextPage    *bool
	}{
		{"NegativeLimit_ReturnsError", SearchRequest{Limit: -1}, "limit must be > 0", 0, false, nil},
		{"NegativeOffset_ReturnsError", SearchRequest{Limit: 1, Offset: -5}, "offset must be > 0", 0, false, nil},
		{"QueryNotFound_ReturnsEmpty", SearchRequest{Query: "DefinitelyNotFound", Limit: 5}, "", 0, false, nil},
		{"Limit1_NextPageLogic", SearchRequest{Limit: 1}, "", 1, false, func() *bool { b := true; return &b }()},
		{"Limit1_OffsetLast_ReturnsEmpty", SearchRequest{Limit: 1, Offset: -1}, "", 0, true, func() *bool { b := false; return &b }()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := c.req
			if c.name == "Limit1_OffsetLast_ReturnsEmpty" {
				if total == 0 {
					t.Skip("Нет данных для теста")
				}
				req.Offset = total
			}
			res, err := sc.FindUsers(req)
			if c.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.expectErr)
			} else {
				require.NoError(t, err)
				assert.Len(t, res.Users, c.expectLen)
				if c.nextPage != nil {
					assert.Equal(t, *c.nextPage, res.NextPage)
				}
			}
		})
	}
}

func TestFindUsers_ServerReturnsInvalidJSON(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"ObjectInsteadOfArray", `{"not_a_user_array": true}`},
		{"NumberInsteadOfArray", `12345`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(c.body))
			}))
			defer ts.Close()
			sc := SearchClient{AccessToken: "test_token", URL: ts.URL}
			_, err := sc.FindUsers(SearchRequest{Limit: 1})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cant unpack result json")
		})
	}
}
