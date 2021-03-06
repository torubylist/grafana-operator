package controller

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tsloughter/grafana-operator/pkg/grafana"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
    "github.com/tidwall/sjson"
	"crypto/sha1"
)

// ConfigMapController watches the kubernetes api for changes to configmaps and
// creates a RoleBinding for that particular configmap.
type ConfigMapController struct {
	configmapInformer cache.SharedIndexInformer
	kclient           *kubernetes.Clientset
	g                 grafana.APIInterface
	hashes            map[string]bool
}

// Run starts the process for listening for configmap changes and acting upon those changes.
func (c *ConfigMapController) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	// When this function completes, mark the go function as done
	defer wg.Done()

	// Increment wait group as we're about to execute a go function
	wg.Add(1)

	// Execute go function

	go c.configmapInformer.Run(stopCh)

	// Wait till we receive a stop signal
	<-stopCh
}

// NewConfigMapController creates a new NewConfigMapController
func NewConfigMapController(kclient *kubernetes.Clientset, g grafana.APIInterface) *ConfigMapController {
	configmapWatcher := &ConfigMapController{hashes: make(map[string]bool)}
	// Create informer for watching ConfigMaps
	ns := g.GetNamespace()
	log.Println("configmap namespace: ", ns)
	configmapInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kclient.Core().ConfigMaps(ns).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.Core().ConfigMaps(ns).Watch(options)
			},
		},
		&v1.ConfigMap{},
		3*time.Minute,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	configmapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: configmapWatcher.CreateDashboards,
	})

	configmapWatcher.kclient = kclient
	configmapWatcher.configmapInformer = configmapInformer
	configmapWatcher.g = g

	return configmapWatcher
}

func (c *ConfigMapController) CreateDashboards(obj interface{}) {
	configmapObj := obj.(*v1.ConfigMap)
	dh, _ := configmapObj.Annotations["grafana.net/dashboards"]
	ds, _ := configmapObj.Annotations["grafana.net/datasource"]
	fd, _ := configmapObj.Annotations["grafana.net/folder"]

	isGrafanaDashboards, _ := strconv.ParseBool(dh)
	isGrafanaDatasource, _ := strconv.ParseBool(ds)
	var folderid int
	if isGrafanaDashboards && fd != "" {
		if id, ok := grafana.Folders[fd]; ok {
			folderid = id
			log.Println(fmt.Sprintf("foldername: %s", fd))
			log.Println(fmt.Sprintf("Getting folder id : %d;folder name: %s;", folderid, fd))
		}else{
			log.Println(fmt.Sprintf("%s is not in foldernames", fd))
		}
	}

	if isGrafanaDashboards || isGrafanaDatasource {
		var err error
		for k, v := range configmapObj.Data {
			//check if the datasource/dashboards already posted.if true, skip, then post.
			vSha := computeSha1(v)
			if c.hashes[vSha] {
				log.Println(fmt.Sprintf("dashboard already exist, %s skipped", k))
				continue
			}
			c.hashes[vSha] = true
			if isGrafanaDatasource {
				log.Printf("Creating datasource : %s;", k)
				err = c.g.CreateDatasource(strings.NewReader(v))
			} else {
				value, _ := sjson.Set(v, "folderId", folderid)
				log.Printf("Creating dashboard : %s;folder id %d", k, folderid)
				err = c.g.CreateDashboard(strings.NewReader(value))
			}
			if err != nil {
				log.Printf("Failed to create %s, %s", err, k)
			} else {
				log.Printf("Created %s", k)
			}
		}
	} else {
		//log.Println(fmt.Sprintf("Skipping configmap: %s", configmapObj.Name))
	}
}

func computeSha1(payload string) string {
	hash := sha1.New()
	hash.Write([]byte(payload))

	return fmt.Sprintf("%x", hash.Sum(nil))
}
