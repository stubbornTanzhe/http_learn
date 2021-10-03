package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"log"
	"net/http"
	"os"
)

func healthz(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	resp := make(map[string]string)
	resp["message"] = "Status OK"
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("Error happened in JSON marshal. Err: %s", err)
	}
	w.Write(jsonResp)
	return
}

func init() {
	flag.Set("logtostderr", "true")
}

func main() {
	flag.Parse()
	defer glog.Flush()

	healthzHandler := http.HandlerFunc(healthz)
	http.Handle("/healthz", middlewareHandler(healthzHandler))
	http.ListenAndServe(":8090", nil)
}

type StatusRecorder struct {
	http.ResponseWriter
	Status int
}

func (r *StatusRecorder) WriteHeader(status int) {
	r.Status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *StatusRecorder) GetStatus() int {
	return r.Status
}

func middlewareHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &StatusRecorder{
			ResponseWriter: w,
			Status:         200,
		}
		defer func() {
			glog.Infof(fmt.Sprintf("status:%+v", recorder.GetStatus()))
		}()

		recorder.Header().Set("Content-Type", "application/json")
		glog.Info(r.Header)
		//接收客户端 request，并将 request 中带的 header 写入 response header
		for i, v := range r.Header {
			for _, t := range v {
				recorder.Header().Set(i, t)
			}
		}

		//读取当前系统的环境变量中的 VERSION 配置，并写入 response header
		version := os.Getenv("VERSION")
		recorder.Header().Set("VERSION", version)

		//Server 端记录访问日志包括客户端 IP，HTTP 返回码，输出到 server 端的标准输出
		glog.Info(r.RemoteAddr)

		next.ServeHTTP(recorder, r)
	})
}
