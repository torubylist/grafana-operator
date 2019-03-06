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
	"k8s.io/apimachinery/pkg/util/wait"
)

type APIInterface interface {
	SearchDashboard() ([]GrafanaDashboard, error)
	CreateDashboard(dashboardJson io.Reader) error
	DeleteDashboard(slug string) error
	CreateDatasource(datasourceJson io.Reader) error
	SetFolders() error
	UpdateHomePage(hp string) error
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
	Uri   string `json:"uri,omitempty"`
}

type DashboardSpec struct {
	Dashboard GrafanaDashboard `json:"dashboard"`
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
	url := makeUrl(c.BaseUrl, "/api/dashboards/import")
	log.Printf("try to create dashboard %s", url)
	return doRestApis("POST", url, dashboardJSON, c.HTTPClient)
}

//create folder map
func (c *APIClient) SetFolders() error {
	err := c.CreateFolder()
	//if err != nil {
	//		return err
	//}
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

//create folders for grafana
func (c *APIClient) CreateFolder() error {
	c.WaitForGrafanaUp()
	log.Printf("creating %s, %s, %s\n", c.BaseUrl, "/api/folders", c.FolderNames)
	foldernames := strings.Split(c.FolderNames, ",")

	for _, folder := range foldernames {
		f := map[string]string{"title": folder}
		fp, _ := json.Marshal(f)
		log.Println(fmt.Sprintf("creating %s, %s, %s", c.BaseUrl, "/api/folders", fp))
		folderUrl := makeUrl(c.BaseUrl, "/api/folders")
		err := doRestApis("POST", folderUrl, bytes.NewReader(fp), c.HTTPClient)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *APIClient) CreateDatasource(datasourceJSON io.Reader) error {
	log.Printf("try to create %s, %s\n", c.BaseUrl, "/api/datasources")
	datasourceUrl := makeUrl(c.BaseUrl, "/api/datasources")
	return doRestApis("POST", datasourceUrl, datasourceJSON, c.HTTPClient)
}

//set home page to specific dashboard.
func (c *APIClient) UpdateHomePage(hp string) error {
	log.Printf("try to set homepage to  %s\n", hp)

	//get specific dashboard id
	slugUrl := makeUrl(c.BaseUrl, fmt.Sprintf("%s%s","/api/dashboards/db/", hp))
	log.Printf("homepage url %s\n", slugUrl)
	dbid, err := getDashboardId(slugUrl)
	if err != nil {
		log.Printf("Get dashboardid %s failed %s \n.", hp, err)
		return err
	}

	//star the dashboard.
	starUrl := makeUrl(c.BaseUrl, fmt.Sprintf("%s/%d", "/api/user/stars/dashboard", dbid))
	err = starDashboard(starUrl, c.HTTPClient)
	if err != nil {
		log.Printf("Star dashboard %d failed %s \n.", dbid, err)
		return err
	}

	//update preference home page to hp
	prefUrl := makeUrl(c.BaseUrl,"/api/org/preferences")
	err = c.updateHomeDashboard(prefUrl, dbid, c.HTTPClient)
	if err != nil {
		return err
	}
	fmt.Printf("Set homepage to dashboard %s done\n", hp)
	return nil
}

func getDashboardId(dashboardSlugUrl string) (int,error) {
	log.Printf("try to dashboard id from %s\n", dashboardSlugUrl)
	var id int
	var err error
	for i:=0;i<10;i++{
		time.Sleep(6*time.Second)
		id,err = func () (int, error) {
			resp, err := http.Get(dashboardSlugUrl)
			if err != nil {
				log.Printf("get dashboardid failed: %s\n", err)
				return -1, err
			}else if resp.StatusCode != http.StatusOK {
				return -1, errors.New(fmt.Sprintf("Wrong http response status %d", resp.StatusCode))
			}else {
				log.Println("get dashboard page success,then decode resp.body")
			}
			defer resp.Body.Close()
			var dashboard DashboardSpec
			err = json.NewDecoder(resp.Body).Decode(&dashboard)
			log.Printf("dashboard: %v\n", dashboard)
			if err != nil {
				log.Printf("json decode failed. %s", err)
				return -1, err
			}

			return dashboard.Dashboard.Id, nil
		}()
		if id != -1 {
			log.Printf("Already get dashboard id: %d", id)
			break
		}
	}
	return id, err
}

func starDashboard(starUrl string, hClient *http.Client) error {
	log.Printf("try to star dashboard url: %s\n", starUrl)
	err := doRestApis("POST", starUrl, nil, hClient)
	if err != nil {
		return err
	}
	return nil
}

func (c *APIClient)updateHomeDashboard(url string, dashboardId int, hClient *http.Client) error {
	pref := map[string]int{"homeDashboardId": dashboardId}
	prefjson, err := json.Marshal(pref)
	if err != nil {
		log.Printf("json marshal failed. %s", err)
		return err
	}
	log.Println(fmt.Sprintf("Updating preferences %s,%s", url, prefjson))
	err = doRestApis("PUT", url, bytes.NewReader(prefjson), hClient)
	if err != nil {
		return err
	}
	fmt.Printf("Changed home page to dashboardid %d\n", dashboardId)
	return nil
}

func (c *APIClient) WaitForGrafanaUp() error {
	grafanaHealthUrl := fmt.Sprintf("%s/api/health", c.BaseUrl)
	err := wait.Poll(3*time.Second, 10*time.Minute, func() (bool,error) {
		resp, err := http.Get(grafanaHealthUrl)
		grafanaUp := false
		if err != nil {
			log.Printf("Failed to request Grafana Health: %s\n", err)
			return false, err
		} else if resp.StatusCode != 200 {
			log.Printf("Grafana Health returned with %d\n", resp.StatusCode)
		} else {
			grafanaUp = true
		}
		defer resp.Body.Close()

		if grafanaUp {
			return true, nil
		} else {
			log.Println("Trying Grafana Health again in 3s")
			return false, errors.New("grafana not ready, retry later")
		}
	})
	return err
}

func doRestApis(action, url string, dataJSON io.Reader, hClient *http.Client) error {
	log.Println(action, url)
	req, err := http.NewRequest(action, url, dataJSON)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")

	if os.Getenv("GRAFANA_BEARER_TOKEN") != "" {
		req.Header.Add("Authorization", "Bearer "+os.Getenv("GRAFANA_BEARER_TOKEN"))
	}

	return doRequest(hClient, req)
}

func doRequest(hClient *http.Client, req *http.Request) error {
	resp, err := hClient.Do(req)
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
