package satim_test

import (
	"encoding/json"
	"testing"

	"github.com/muandane/go-satim"
)

func FuzzRegisterOrderRequest_Validate(f *testing.F) {
	f.Add(int64(100000), "https://example.com/return", "https://example.com/fail", "FR", int64(1234567890), "Order #1", int(1800))
	f.Add(int64(0), "", "", "EN", int64(0), "", int(0))
	f.Add(int64(-50), "invalid-url", "invalid-url", "AR", int64(999), "desc", int(500))

	f.Fuzz(func(_ *testing.T, amount int64, returnURL, failURL, lang string, orderNum int64, desc string, timeout int) {
		req := satim.RegisterOrderRequest{
			AmountMinor:        amount,
			ReturnURL:          returnURL,
			FailURL:            failURL,
			Language:           satim.Language(lang),
			OrderNumber:        orderNum,
			Description:        desc,
			SessionTimeoutSecs: timeout,
		}
		_ = req.Validate()
	})
}

func FuzzRefundRequest_Validate(f *testing.F) {
	f.Add("ord-123456", int64(50000), "FR")
	f.Add("", int64(0), "")
	f.Add("ord-abc", int64(-100), "DE")

	f.Fuzz(func(_ *testing.T, orderID string, amount int64, lang string) {
		req := satim.RefundRequest{
			OrderID:     orderID,
			AmountMinor: amount,
			Language:    satim.Language(lang),
		}
		_ = req.Validate()
	})
}

func FuzzOrderStatusResponse_UnmarshalJSON(f *testing.F) {
	f.Add([]byte(`{"orderId":"ord-123","OrderStatus":"2","ErrorCode":"0","amount":"150000","currency":"012"}`))
	f.Add([]byte(`{"orderId":"ord-456","OrderStatus":2,"ErrorCode":0,"amount":150000}`))
	f.Add([]byte(`{"errorCode":"5","errorMessage":"Access denied"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`invalid json`))

	f.Fuzz(func(_ *testing.T, data []byte) {
		var resp satim.OrderStatusResponse
		_ = json.Unmarshal(data, &resp)
	})
}
