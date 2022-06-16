package stub

import (
	"strings"

	"github.com/sirupsen/logrus"
)

const annotationDomainSeparator = "/"
const annotationSubDomainSeparator = "."

func parseMetrics(annotations map[string]string, deploymentName string) string {

	var metrics string

	for metricKey, metricValue := range annotations {
		keys := strings.Split(metricKey, annotationDomainSeparator)
		if len(keys) != 2 {
			logrus.Errorf("Metric annotation for deployment %v is invalid: %v", deploymentName, metricKey)
			return metrics
		}
		metricSubDomains := strings.Split(keys[0], annotationSubDomainSeparator)
		if len(metricSubDomains) < 2 {
			logrus.Errorf("Metric annotation for deployment %v is invalid: %v", deploymentName, metricKey)
			return metrics
		}

		switch metricSubDomains[0] {
		case "yamlfile":
			metrics = metricValue
		}

	}

	return metrics
}
