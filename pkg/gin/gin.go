package gin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/huisebug/k8simage-operator/pkg/mysql"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ContainerInfo struct {
	Namespace      string
	Name           string
	ContainerName  string
	ContainerIndex int
	ContainerImage string
}

var clientset *kubernetes.Clientset

func GinRun() {

	r := gin.Default()
	v1 := r.Group("/api/v1")

	// 探测
	v1.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})
	// 更新deployment
	v1.POST("/update", func(c *gin.Context) {

		b, _ := c.GetRawData() // 从c.Request.Body读取请求数据
		// 定义map或结构体
		CI := &ContainerInfo{}
		// 反序列化
		_ = json.Unmarshal(b, CI)
		fmt.Println(CI)
		// update集群中的deployment
		err := HandleUpdate(*CI)
		fmt.Println(err)
		if err != nil {
			switch err.Error() {
			case "index":
				c.JSON(http.StatusOK, gin.H{
					"message": "容器索引位置和容器名不一致",
				})
			case "noneed":
				c.JSON(http.StatusOK, gin.H{
					"message": "容器不需要更新",
				})

			default:
				c.JSON(http.StatusOK, gin.H{
					"message": "传递信息有误",
				})
			}
		} else {
			c.JSON(http.StatusOK, gin.H{
				"message": "update成功",
			})
		}

	})
	// 删除deployment信息
	v1.POST("/delete", func(c *gin.Context) {
		b, _ := c.GetRawData() // 从c.Request.Body读取请求数据
		// 定义map或结构体
		ksi := &mysql.K8sServerImage{}
		// 反序列化
		_ = json.Unmarshal(b, ksi)
		fmt.Println(ksi)
		// 删除数据库中对应的deployment信息
		if ksi.IsNowRun {
			c.JSON(http.StatusOK, gin.H{
				"message": "此镜像版本是正在运行的，不允许删除",
			})
		} else {
			// 执行"删除"操作
			switch ksi.DeleteRecord(ksi.Namespace + "_" + ksi.Name) {
			case int64(1):
				c.JSON(http.StatusOK, gin.H{
					"message": "删除完毕",
				})
			default:
				c.JSON(http.StatusOK, gin.H{
					"message": "删除失败",
				})
			}
		}
	})

	// 获取所有的deployment信息
	v1.GET("/getdeploy", func(c *gin.Context) {

		tables := mysql.GetAllTables()
		// fmt.Println(tables)
		// 查询对应命名空间下的deployment是还存在
		tables = func(tables []string) []string {
			var tablesformat []string
			for _, table := range tables {
				tablesplit := strings.Split(table, "_")
				_, err := IsExistDeeployment(tablesplit[0], tablesplit[1])
				if err == nil {
					tablesformat = append(tablesformat, table)
				}
			}
			return tablesformat
		}(tables)
		ksi := mysql.K8sServerImage{}
		alldata := ksi.GetAllDatas(tables)
		// fmt.Println(alldata)
		c.JSON(200, &alldata)

	})
	// 获取数据库中所有的表格名（ns_deployment格式）
	v1.GET("/list", func(c *gin.Context) {
		tables := mysql.GetAllTables()
		// fmt.Println(tables)
		c.JSON(http.StatusOK, &tables)

	})
	// 获取单个deployment信息
	v1.GET("/get", func(c *gin.Context) {
		table := c.Query("table")
		fmt.Println(table)
		yes, _ := regexp.MatchString("[a-z0-9]([-a-z0-9]*[a-z0-9])?_[a-z0-9]([-a-z0-9]*[a-z0-9])?", table)
		if yes {
			ksi := mysql.K8sServerImage{}
			datas := ksi.GetDatas(table)
			// fmt.Println(alldata)
			c.JSON(200, gin.H{
				table: datas,
			})
		} else {
			c.JSON(200, gin.H{
				"message": "传递信息不符合规范，需传递：?table=Namespace名称_Deployment名称",
			})
		}

	})

	// 读取Yaml文件位置并返回
	v1.GET("/yamlfile", func(c *gin.Context) {
		//根据文件路径读取返回流文件 参数url
		yamlfilePath := c.Query("url")

		if bytes, err := ioutil.ReadFile(yamlfilePath); err != nil {
			c.JSON(200, gin.H{
				"message": "传递信息不符合规范，需传递：?table=Namespace名称_Deployment名称",
			})
		} else {
			c.JSON(200, gin.H{
				"yamlfile": string(bytes),
			})
		}

	})
	r.Run(":8888")
}

func Createk8sClient() *kubernetes.Clientset {
	// 创建集群内配置
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Println(err)
	}
	// 创建客户端
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println(err)
	}
	return clientset
}

//查询对应命名空间下的deployment是否存在
func IsExistDeeployment(namespace, name string) (*v1.Deployment, error) {
	clientset = Createk8sClient()
	deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return deployment, nil
}

func HandleUpdate(CI ContainerInfo) error {
	clientset = Createk8sClient()
	//查询对应命名空间下的deployment是否存在
	deployment, err := IsExistDeeployment(CI.Namespace, CI.Name)
	if err != nil {
		return err
	}
	index := CI.ContainerIndex

	if index >= len(deployment.Spec.Template.Spec.Containers) {
		return errors.New("index")
	}

	if deployment.Spec.Template.Spec.Containers[index].Name == CI.ContainerName {
		if deployment.Spec.Template.Spec.Containers[index].Image == CI.ContainerImage {
			return errors.New("noneed")
		}
		deployment.Spec.Template.Spec.Containers[index].Image = CI.ContainerImage
	} else {
		return errors.New("index")
	}

	// 更新
	_, err = clientset.AppsV1().Deployments(CI.Namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}
