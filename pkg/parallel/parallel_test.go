package parallel

import (
	"reflect"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		want *Client
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("New() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_do(t *testing.T) {
	type args struct {
		target string
	}
	tests := []struct {
		name    string
		args    args
		wantR   res
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, err := do(tt.args.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("do() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotR, tt.wantR) {
				t.Errorf("do() = %v, want %v", gotR, tt.wantR)
			}
		})
	}
}

func TestClient_GetParallelHosts(t *testing.T) {
	type args struct {
		targets []string
	}
	tests := []struct {
		name    string
		c       *Client
		args    args
		wantErr bool
	}{
		// {name: "single", c: New(), args: args{targets: []string{"https://google.com"}}, wantErr: false},
		{name: "many", c: New(), args: args{targets: []string{
			"https://test.sonybn.co.jp",
			"https://www.personyms.com",
			"https://www.wigtypes.com",
			"https://*.rsnx.ru",
			"https://125.7.72.94",
			"https://www.beautyofnewyork.com",
			"https://beautyofnewyork.com",
			"https://dds-1027148-6249.host4g.ru",
			"https://pmailbackup01.stg.personyms.ninja",
			"https://kiusys.com",
			"https://rsnx.ru",
			"https://*.kiusys.com",
			"https://pmail01.stg.personyms.ninja",
			"https://git.devops.sonyselect.cn",
			"https://193.178.208.1",
			"https://www.personyms.ninja",
			"https://sonyred.net",
			"https://wigtypes.comapi.playstation.com",
			"https://catalog.e1.api.playstation.com",
			"https://commerce.e1.api.playstation.com",
			"https://ems-delivery.e1.api.playstation.com",
			"https://entitlements.e1.api.playstation.com",
			"https://events.sp.api.playstation.com",
			"https://cba.api.playstation.com",
			"https://cis.api.playstation.com",
			"https://accounts.dev.api.playstation.com",
			"https://dms.api.playstation.com",
			"https://laco.dms.api.playstation.com",
			"https://google.com"}}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.c.GetParallelHosts(tt.args.targets); (err != nil) != tt.wantErr {
				t.Errorf("Client.GetParallelHosts() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
