package stub

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	repyaml "github.com/ghodss/yaml"
	"github.com/huisebug/k8simage-operator/pkg/mysql"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const yamlfileAnnotationPrefix = "yamlfile.huisebug.io"

const annotationRegExpString = "yamlfile\\.huisebug\\.io\\/[a-zA-Z\\.]+"

func NewHandler(client client.Client) *K8sImage {
	r, _ := regexp.Compile(annotationRegExpString)
	return &K8sImage{
		annotationRegExp: r,
		client:           client,
	}
}

type K8sImage struct {
	annotationRegExp *regexp.Regexp
	client           client.Client
}

type Inspectyamlfile struct {
	Kind          string
	Name          string
	Namespace     string
	Containername string
}

func (h *K8sImage) filterAnnotations(annotations map[string]string) map[string]string {
	Annotations := make(map[string]string)
	for key, value := range annotations {
		if h.annotationRegExp.MatchString(key) {
			Annotations[key] = value
		}
	}
	return Annotations
}

func (h *K8sImage) HandleReplicaSet(
	ctx context.Context,
	deployment *appsv1.Deployment) error {
	var server mysql.K8sServerImage
	server.Name = deployment.Name
	server.Namespace = deployment.Namespace
	logrus.Infof("deployment.Name : %v", deployment.Name)
	Annotations := h.filterAnnotations(deployment.Annotations)
	if len(Annotations) > 0 {
		yamlfile := parseMetrics(Annotations, deployment.Name)
		logrus.Infof("Yamlfile annotations found on %v", deployment.Kind)
		logrus.Infof("%v", yamlfile)
		server.Yamlfile = yamlfile

	}
	// 替换本地存放的yaml文件的image信息

	var containerimagelist []string
	// 存储到数据库中
	for index, container := range deployment.Spec.Template.Spec.Containers {
		logrus.Infof("deployment.container.%v,Image  : %v", index, container.Image)

		server.ContainerIndex = index
		server.ContainerName = container.Name
		server.Image = container.Image
		containerimagelist = append(containerimagelist, container.Image)

		mysqltablename := deployment.Namespace + "_" + deployment.Name
		server.Run(mysqltablename)
	}
	if server.Yamlfile != "" {
		newcontent := ReadYamlSedConfig(server.Yamlfile, deployment, containerimagelist)
		// fmt.Println("newcontent", string(newcontent))
		f, err := os.OpenFile(server.Yamlfile, os.O_WRONLY|os.O_TRUNC, 0666)
		if err != nil {
			fmt.Println("文件打开失败")
		}
		defer f.Close()
		f.Write(newcontent)

	}

	return nil
}

// 返回yamlfile文件中的Container字典中对应索引index对应的image字段
func ReadYamlSedConfig(yamlfile string, rundeployment *appsv1.Deployment, containerimagelist []string) []byte {

	var yamlfilebyteslice []byte
	// bufio包需要io.Reader
	var r io.Reader
	var err error
	r, err = os.Open(yamlfile)
	if err != nil {
		fmt.Println(err)
	}
	br := bufio.NewReader(r)
	// 因为k8s的yaml支持分隔符---,想要读取到分隔后的其他yaml，所以需要进行多次for循环
	YR := yaml.NewYAMLReader(br)
	// i := 1
	for {
		// 分割读取后得到对应分隔符后的yaml字节切片
		yamlbyteslice, err := YR.Read()
		if err != nil {
			fmt.Println("err: ", err)
			break
		}
		// 使用k8s yaml包中提供的yaml转json
		// 下面的yaml.Unmarshal是可以将yaml中的直接反序列到deployment中的，但是因为deployment的结构体定义的是json处理声明，
		// 替换对应字段后再转回yaml会存在deployment未定义的垃圾数据，所以只能先转为json
		// yaml.Unmarshal(kindyaml, deployment)
		yamljsonbyteslice, err := yaml.ToJSON(yamlbyteslice)
		if err != nil {
			fmt.Println("err: ", err)
			break
		}
		// fmt.Println(string(yamljsonbyteslice))
		// 将yaml转为json后的数据反序列化到deployment结构体中
		deployment := &appsv1.Deployment{}
		err = json.Unmarshal(yamljsonbyteslice, deployment)
		if err != nil {
			fmt.Println("反序列化失败")
		} else {
			// 仅处理Deployment
			if deployment.Kind == "Deployment" {
				// 处理未填写namespace时，默认命名空间是default
				if deployment.Namespace == "" {
					deployment.Namespace = "default"
				}
				// 必须对应信息相同才进行内容替换
				if deployment.Namespace == rundeployment.Namespace && deployment.Name == rundeployment.Name {
					// 处理image
					for index, image := range containerimagelist {
						deployment.Spec.Template.Spec.Containers[index].Image = image
					}
					// 将处理好了的deployment序列化为json，这时就会根据deployment的json声明进行序列化，就不会有值为null的字段
					newyamljsonbyteslice, err := json.Marshal(deployment)
					if err != nil {
						fmt.Println("序列化失败")
					} else {
						// 再使用json转yaml的方式转回yaml
						newyamlbyteslice, err := repyaml.JSONToYAML(newyamljsonbyteslice)
						if err != nil {
							fmt.Printf("err: %v\n", err)
						} else {

							newyamlbyteslice = HandleBlankLines(newyamlbyteslice)
							// fmt.Println(string(newyamlbyteslice))
							yamlfilebyteslice = append(yamlfilebyteslice, newyamlbyteslice...)
							yamlfilebyteslice = append(yamlfilebyteslice, []byte("\n---\n")...)
						}
					}

				} else {
					yamlbyteslice = HandleBlankLines(yamlbyteslice)
					yamlfilebyteslice = append(yamlfilebyteslice, yamlbyteslice...)
					yamlfilebyteslice = append(yamlfilebyteslice, []byte("\n---\n")...)
					continue
				}

			} else {
				yamlbyteslice = HandleBlankLines(yamlbyteslice)
				yamlfilebyteslice = append(yamlfilebyteslice, yamlbyteslice...)
				yamlfilebyteslice = append(yamlfilebyteslice, []byte("\n---\n")...)
				continue
			}

		}
	}
	return yamlfilebyteslice
}

// 处理空行
func HandleBlankLines(src []byte) []byte {
	s := string(src)
	d := regexp.MustCompile(`[\t\r\n]+`).ReplaceAllString(strings.TrimSpace(s), "\n")
	return []byte(d)
}
