#!/bin/bash
if [ ! -f "/usr/bin/htpasswd" ]; then
yum -y install httpd-tools
fi
# 生成的文件名称必须为auth，不允许更改为其他名称，$1为用户名  $2为密码， $1后面需要在ingress中做用户名检验
htpasswd -bc /etc/kubernetes/k8simage-operator/auth $1 $2
kubectl delete secret -n k8simage-operator-system nginx-ingress-auth >/dev/null 2>&1
# secret的名称可以随便定义
kubectl -n k8simage-operator-system create secret generic nginx-ingress-auth --from-file=/etc/kubernetes/k8simage-operator/auth
