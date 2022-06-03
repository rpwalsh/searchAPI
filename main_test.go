package main

import (
	"database/sql"
	"github.com/lib/pq"
	"io"
	"net/http"
	"reflect"
	"testing"
)

func TestOpenConnection(t *testing.T) {
	tests := []struct {
		name string
		want *sql.DB
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OpenConnection(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OpenConnection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_apiAuth(t *testing.T) {
	tests := []struct {
		name string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiAuth()
		})
	}
}

func Test_elasticListener(t *testing.T) {
	tests := []struct {
		name string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elasticListener()
		})
	}
}

func Test_elasticNotify(t *testing.T) {
	type args struct {
		l *pq.Listener
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elasticNotify(tt.args.l)
		})
	}
}

func Test_elasticReq(t *testing.T) {
	type args struct {
		method string
		id     string
		reader io.Reader
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := elasticReq(tt.args.method, tt.args.id, tt.args.reader); got != tt.want {
				t.Errorf("elasticReq() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_elasticWrite(t *testing.T) {
	type args struct {
		message Message
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elasticWrite(tt.args.message)
		})
	}
}

func Test_esRequired(t *testing.T) {
	tests := []struct {
		name string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esRequired()
		})
	}
}

func Test_handleRequests(t *testing.T) {
	tests := []struct {
		name string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handleRequests()
		})
	}
}

func Test_homePage(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homePage(tt.args.w, tt.args.r)
		})
	}
}

func Test_httpReq(t *testing.T) {
	type args struct {
		method string
		url    string
		reader io.Reader
	}
	tests := []struct {
		name string
		args args
		want *http.Response
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := httpReq(tt.args.method, tt.args.url, tt.args.reader); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("httpReq() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isErrorHTTPCode(t *testing.T) {
	type args struct {
		resp *http.Response
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isErrorHTTPCode(tt.args.resp); got != tt.want {
				t.Errorf("isErrorHTTPCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newDB(t *testing.T) {
	tests := []struct {
		name string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newDB()
		})
	}
}

func Test_returnAllEmployees(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnAllEmployees(tt.args.w, tt.args.r)
		})
	}
}

func Test_returnAllPairedTasks(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnAllPairedTasks(tt.args.w, tt.args.r)
		})
	}
}

func Test_returnEmployeesByTask_esapi(t *testing.T) {
	type args struct {
		w  http.ResponseWriter
		hr *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnEmployeesByTask_esapi(tt.args.w, tt.args.hr)
		})
	}
}

func Test_returnPairedTask_byID(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnPairedTask_byID(tt.args.w, tt.args.r)
		})
	}
}

func Test_returnPublicTasks(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnPublicTasks(tt.args.w, tt.args.r)
		})
	}
}

func Test_returnSingleByUUID(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnSingleByUUID(tt.args.w, tt.args.r)
		})
	}
}

func Test_returnSingleEmpUUID_esapi(t *testing.T) {
	type args struct {
		w  http.ResponseWriter
		hr *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnSingleEmpUUID_esapi(tt.args.w, tt.args.hr)
		})
	}
}

func Test_returnSingleEmployee(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnSingleEmployee(tt.args.w, tt.args.r)
		})
	}
}

func Test_returnSingleTask(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnSingleTask(tt.args.w, tt.args.r)
		})
	}
}

func Test_returnSingleTask_esapi(t *testing.T) {
	type args struct {
		w  http.ResponseWriter
		hr *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnSingleTask_esapi(tt.args.w, tt.args.hr)
		})
	}
}

func Test_searchESAPI(t *testing.T) {
	tests := []struct {
		name string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchESAPI()
		})
	}
}

func Test_setup(t *testing.T) {
	tests := []struct {
		name string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup()
		})
	}
}
