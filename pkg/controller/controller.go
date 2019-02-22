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
)

// ConfigMapController watches the kubernetes api for changes to configmaps and
// creates a RoleBinding for that particular configmap.
type ConfigMapController struct {
	configmapInformer cache.SharedIndexInformer
	kclient           *kubernetes.Clientset
	g                 grafana.APIInterface
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
	configmapWatcher := &ConfigMapController{}

	// Create informer for watching ConfigMaps
	configmapInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kclient.Core().ConfigMaps(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.Core().ConfigMaps(metav1.NamespaceAll).Watch(options)
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
		if folder, ok := grafana.Folders[fd]; ok {
			folderid = folder
			log.Println(fmt.Sprintf("foldername: %s", fd))
			log.Println(fmt.Sprintf("Getting folder id : %d;folder name: %s;", folderid, fd))
		}else{
			log.Println(fmt.Sprintf("%s is not in foldernames", fd))
		}
	}

	if isGrafanaDashboards || isGrafanaDatasource {
		var err error
		for k, v := range configmapObj.Data {
			if isGrafanaDatasource {
				log.Println(fmt.Sprintf("Creating datasource : %s;", k))
				err = c.g.CreateDatasource(strings.NewReader(v))
			} else {
				value, _ := sjson.Set(v, "folderId", folderid)
				log.Println(fmt.Sprintf("Creating dashboard : %s;", k))
				err = c.g.CreateDashboard(strings.NewReader(value))
			}

			if err != nil {
				log.Println(fmt.Sprintf("Failed to create %s, %s", err, k))
			} else {
				log.Println(fmt.Sprintf("Created %s", k))
			}
		}
	} else {
		//log.Println(fmt.Sprintf("Skipping configmap: %s", configmapObj.Name))
	}
}

