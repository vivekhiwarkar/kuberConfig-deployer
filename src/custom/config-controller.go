package custom

import (
	"context"
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	informersv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type controller struct {
	clientset            kubernetes.Interface
	configMapLister      listersv1.ConfigMapLister
	workQueue            workqueue.RateLimitingInterface
	configMapCacheSynced cache.InformerSynced
}

func CustomController(clientset kubernetes.Interface, configMapInformer informersv1.ConfigMapInformer) *controller {
	controller := &controller{
		clientset:            clientset,
		configMapLister:      configMapInformer.Lister(),
		workQueue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "konfig-deployer"),
		configMapCacheSynced: configMapInformer.Informer().HasSynced,
	}

	configMapInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: controller.handleAdd,
		},
	)

	return controller
}

func (c *controller) Run(stopCh <-chan struct{}) {
	fmt.Println("Starting Custom Controller....")
	if !cache.WaitForCacheSync(stopCh, c.configMapCacheSynced) {
		fmt.Println("Waiting for the cache to be synced....")
	}

	go wait.Until(c.worker, 1*time.Second, stopCh)

	<-stopCh
}

func (c *controller) worker() {
	for c.processItem() {
	}
}

func (c *controller) processItem() bool {
	item, shutdown := c.workQueue.Get()
	if shutdown {
		return false
	}

	defer c.workQueue.Forget(item)

	key, err := cache.MetaNamespaceKeyFunc(item)
	if err != nil {
		fmt.Println("Error in cache.MetaNamespaceKeyFunc(): ", err.Error())
	}

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		fmt.Println("Error in cache.SplitMetaNamespaceKey(): ", err.Error())
	}

	err = c.createDeployment(ns, name)
	if err != nil {
		fmt.Println("Error in c.createDeployment(): ", err.Error())
		return false
	}
	return true
}

func (c *controller) createDeployment(ns, name string) error {
	ctx := context.Background()

	configMap, err := c.configMapLister.ConfigMaps(ns).Get(name)
	if err != nil {
		fmt.Println("Error in c.configMapLister.ConfigMaps(ns).Get(): ", err.Error())
	}

	if val, ok := configMap.Labels["app"]; ok && val == "auto-deployment" {

		labelSelector := metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "auto-deployment"},
		}
		depReplicasInt64, err := strconv.ParseInt(configMap.Data["DeploymentReplicas"], 10, 64)
		if err != nil {
			fmt.Println("Error in strconv.ParseInt(configMap.Data['DeploymentReplicas']): ", err.Error())
		}
		depReplicas := int32(depReplicasInt64)

		deployment := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMap.Data["DeploymentName"],
				Namespace: ns,
				Labels:    map[string]string{"app": "auto-deployment"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &depReplicas,
				Selector: &labelSelector,
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "auto-deployment"},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  "auto-deployment-container",
								Image: configMap.Data["DeploymentImage"],
							},
						},
					},
				},
			},
		}

		_, err = c.clientset.AppsV1().Deployments(ns).Create(ctx, &deployment, metav1.CreateOptions{})
		if err != nil {
			fmt.Println("Error in c.clientset.AppsV1().Deployments(ns).Create(ctx, &deployment, metav1.CreateOptions{}) : ", err.Error())
		}

		fmt.Printf("Deployment %s has been created", configMap.Data["DeploymentName"])
	}

	return nil
}

func (c *controller) handleAdd(obj interface{}) {
	c.workQueue.Add(obj)
}
