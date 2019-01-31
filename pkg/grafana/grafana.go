// Copyright 2016 The prometheus-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grafana

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type APIInterface interface {
	SearchDashboard() ([]GrafanaDashboard, error)
	CreateDashboard(dashboardJson io.Reader) error
	DeleteDashboard(slug string) error
	CreateDatasource(datasourceJson io.Reader) error
	SetFolders() error
}

type Folder struct {
	ID    int    `json: "id,omitempty"`
	Uid   string `json: "uid,omitempty"`
	Title string `json: "title,omitempty"`
}

type APIClient struct {
	BaseUrl     *url.URL
	FolderNames string
	HTTPClient  *http.Client
}

type GrafanaDashboard struct {
	Id    int    `json:"id"`
	Title string `json:"title"`
	Uri   string `json:"uri"`
}

var Folders map[string]int

func (d *GrafanaDashboard) Slug() string {
	// The uri in the search result contains the slug.
	// http://docs.grafana.org/v3.1/http_api/dashboard/#search-dashboards
	return strings.TrimPrefix(d.Uri, "db/")
}

func (c *APIClient) SearchDashboard() ([]GrafanaDashboard, error) {
	searchUrl := makeUrl(c.BaseUrl, "/api/search")
	resp, err := c.HTTPClient.Get(searchUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	searchResult := make([]GrafanaDashboard, 0)
	err = json.NewDecoder(resp.Body).Decode(&searchResult)
	if err != nil {
		return nil, err
	}

	return searchResult, nil
}

func (c *APIClient) DeleteDashboard(slug string) error {
	deleteUrl := makeUrl(c.BaseUrl, "/api/dashboards/db/"+slug)
	req, err := http.NewRequest("DELETE", deleteUrl, nil)
	if err != nil {
		return err
	}
	return doRequest(c.HTTPClient, req)
}

func (c *APIClient) CreateDashboard(dashboardJSON io.Reader) error {
	log.Println(fmt.Sprintf("creating to create %s, %s", c.BaseUrl, "/api/dashboards/import"))
	return doPost(makeUrl(c.BaseUrl, "/api/dashboards/import"), dashboardJSON, c.HTTPClient)
}

func (c *APIClient) CreateFolder() error {
	c.WaitForGrafanaUp()
	log.Println(fmt.Sprintf("creating %s, %s, %s", c.BaseUrl, "/api/folders", c.FolderNames))
	foldernames := strings.Split(c.FolderNames, ",")

	for _, folder := range foldernames {
		f := map[string]string{"title": folder}
		fp, _ := json.Marshal(f)
		log.Println(fmt.Sprintf("creating %s, %s, %s", c.BaseUrl, "/api/folders", fp))
		err := doPost(makeUrl(c.BaseUrl, "/api/folders"), bytes.NewReader(fp), c.HTTPClient)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *APIClient) SetFolders() error {
	err := c.CreateFolder()
	//if err != nil {
	//		return err
	//	}
	log.Println(fmt.Sprintf("try to get folders %s, %s", c.BaseUrl, "/api/folders"))
	folderUrl := makeUrl(c.BaseUrl, "/api/folders")
	resp, err := c.HTTPClient.Get(folderUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	folders := make([]Folder, 0)
	err = json.NewDecoder(resp.Body).Decode(&folders)
	if err != nil {
		return errors.New("error marshal for folders")
	}
	Folders = make(map[string]int, 3)
	for _, f := range folders {
		Folders[f.Title] = f.ID
	}
	log.Println(Folders)
	return nil
}

func (c *APIClient) CreateDatasource(datasourceJSON io.Reader) error {
	log.Println(fmt.Sprintf("Failed to create %s, %s", c.BaseUrl, "/api/datasources"))
	return doPost(makeUrl(c.BaseUrl, "/api/datasources"), datasourceJSON, c.HTTPClient)
}

func doPost(url string, dataJSON io.Reader, c *http.Client) error {
	req, err := http.NewRequest("POST", url, dataJSON)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")

	if os.Getenv("GRAFANA_BEARER_TOKEN") != "" {
		req.Header.Add("Authorization", "Bearer "+os.Getenv("GRAFANA_BEARER_TOKEN"))
	}

	return doRequest(c, req)
}

func (c *APIClient) WaitForGrafanaUp() error {
	grafanaHealthUrl := fmt.Sprintf("%s/api/health", c.BaseUrl)
	for {
		resp, err := http.Get(grafanaHealthUrl)
		grafanaUp := false
		if err != nil {
			log.Printf("Failed to request Grafana Health: %s", err)
		} else if resp.StatusCode != 200 {
			log.Printf("Grafana Health returned with %d", resp.StatusCode)
		} else {
			grafanaUp = true
		}

		if grafanaUp {
			return nil
		} else {
			log.Println("Trying Grafana Health again in 10s")
			time.Sleep(10 * time.Second)
		}
	}
	return errors.New("grafana is not ready")
}

func doRequest(c *http.Client, req *http.Request) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected status code returned from Grafana API (got: %d, expected: 200, msg:%s)", resp.StatusCode, resp.Status)
	}
	return nil
}

type Clientset struct {
	BaseUrl    *url.URL
	HTTPClient *http.Client
}

func New(baseUrl *url.URL, folderNames string) *APIClient {
	return &APIClient{
		BaseUrl:     baseUrl,
		FolderNames: folderNames,
		HTTPClient:  http.DefaultClient,
	}
}

func makeUrl(baseURL *url.URL, endpoint string) string {
	result := *baseURL

	result.Path = path.Join(result.Path, endpoint)

	return result.String()
}
