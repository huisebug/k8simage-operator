package mysql

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"gorm.io/driver/mysql"

	"gorm.io/gorm"
)

var db2 = new(gorm.DB)
var err error
var MYSQL_HOST, MYSQL_ROOT_PASSWORD string
var wg sync.WaitGroup

type K8sServerImage struct {
	Namespace      string
	Name           string
	ContainerIndex int
	ContainerName  string `gorm:"primaryKey"`
	Image          string `gorm:"primaryKey"`
	IsNowRun       bool   `gorm:"default:true"`
	Yamlfile       string

	CreatedAt time.Time `gorm:"autoCreateTime;column:created_at;<-:create" json:"created_at,omitempty"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;column:updated_at" json:"update_at,omitempty"`
	Deleted   gorm.DeletedAt
}

func init() {
	log.Println("开始连接数据库")
	if os.Getenv("MYSQL_HOST") != "" && os.Getenv("MYSQL_ROOT_PASSWORD") != "" {
		MYSQL_HOST = os.Getenv("MYSQL_HOST")
		MYSQL_ROOT_PASSWORD = os.Getenv("MYSQL_ROOT_PASSWORD")
	} else {
		MYSQL_HOST = "k8simage-operator-controller-manager-mysql"
		MYSQL_ROOT_PASSWORD = "mysql@u214Pp178FQ"
		// MYSQL_HOST = "10.240.53.4"
		// MYSQL_ROOT_PASSWORD = "cd2020@Antiy"
	}
	dsn := fmt.Sprintf("root:%s@tcp(%s)/k8simage?&parseTime=True&loc=Local", MYSQL_ROOT_PASSWORD, MYSQL_HOST)

	// 6次尝试连接机会,都失败就卡住进程
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		for i := 0; i < 6; i++ {
			db2, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
			if err != nil {
				log.Println("db2 open mysql failed,", err)
			} else {
				wg.Done()
				break
			}
			time.Sleep(30 * time.Second)
		}

	}(&wg)
	wg.Wait()

}

func (server *K8sServerImage) Run(tablename string) {
	db2.Table(tablename).AutoMigrate(server)
	result := db2.Table(tablename).Create(server)
	if result.Error != nil {
		result1 := db2.Table(tablename).Model(server).Where("image", server.Image).Updates(server)

		//打印返回插入记录的条数
		fmt.Println("更新数据状态: ", result1.RowsAffected)
	} else {
		fmt.Println("新数据插入状态: ", result.RowsAffected)
	}

	// gorm无法使用where表达式!=，所以只有使用原生sql,又因为使用？占位符，传入时会有单引号进行框起，表名不能有单引号，所以使用字符串拼接的写法
	// 当前运行的image新增或者更新后，其他is_now_run就设置为false，不存储老版本的yaml信息，在数据库中就是0
	result2 := db2.Exec("UPDATE `"+tablename+"` SET is_now_run = ?, yamlfile = '' WHERE image != ? AND container_name = ?", false, server.Image, server.ContainerName)
	fmt.Println("更新is_now_run数据状态: ", result2.RowsAffected)
}

func GetAllTables() []string {
	var tables []string
	// 原生 SQL

	rows, _ := db2.Raw("show tables").Rows()
	defer rows.Close()
	for rows.Next() {
		var table string
		rows.Scan(&table)
		tables = append(tables, table)

	}

	return tables
}

func (server *K8sServerImage) GetDatas(table string) []K8sServerImage {

	ary := []K8sServerImage{}
	db2.Table(table).Model(server).Find(&ary)

	return ary
}

func (server *K8sServerImage) GetAllDatas(tables []string) map[string][]K8sServerImage {
	alldata := make(map[string][]K8sServerImage)
	for _, table := range tables {
		data := server.GetDatas(table)
		alldata[table] = data
	}
	return alldata
}

func (server *K8sServerImage) DeleteRecord(table string) int64 {
	result := db2.Table(table).Delete(&server)
	return result.RowsAffected
}
