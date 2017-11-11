package wxpay

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/relax-space/go-kit/random"

	"github.com/relax-space/go-kit/data"
	"github.com/relax-space/go-kit/httpreq"

	"github.com/relax-space/go-kit/base"
	"github.com/relax-space/go-kit/sign"
)

func LoopQuery(reqDto ReqQueryDto, custDto ReqCustomerDto, limit, interval int) (queryResult map[string]interface{}, err error) {
	count := limit / interval
	waitTime := time.Duration(interval) * time.Second //2s
	for index := 0; index < count; index++ {
		queryResult, err = Query(reqDto, custDto)
		if err == nil { // 1. request wechat query api success
			tradeStatusObj, ok := queryResult["trade_state"]
			if !ok { //1.1 wechat query api response result is exception
				time.Sleep(waitTime)
				continue
			}
			tradeStatus := tradeStatusObj.(string)
			switch {
			case tradeStatus == "SUCCESS": //1.2 pay success
				return
			case tradeStatus == "CLOSED" || tradeStatus == "REFUND" || tradeStatus == "REVOKED" || tradeStatus == "NOTPAY" || tradeStatus == "PAYERROR":
				err = errors.New("wechat pay failure")
				return //1.3 pay failure
			case tradeStatus == "USERPAYING": //1.4 pay unknown
				if index < count {
					time.Sleep(waitTime)
					continue
				}
			}
		} else { //2. request wechat query api failure
			time.Sleep(waitTime)
			continue
		}
	}
	err = errors.New(MESSAGE_OVERTIME)
	return
}

func Pay(reqDto ReqPayDto, custDto ReqCustomerDto) (result map[string]interface{}, err error) {
	wxPayData := BuildCommonparam(reqDto.ReqBaseDto)

	outTradeNo := reqDto.OutTradeNo
	if len(outTradeNo) == 0 {
		outTradeNo = random.Uuid(PRE_OUTTRADENO)
	}
	SetValue(wxPayData, "out_trade_no", outTradeNo)
	SetValue(wxPayData, "body", reqDto.Body)
	SetValue(wxPayData, "total_fee", reqDto.TotalFee)
	SetValue(wxPayData, "auth_code", reqDto.AuthCode)
	SetValue(wxPayData, "device_info", reqDto.DeviceInfo)

	SetValue(wxPayData, "detail", reqDto.Detail)
	SetValue(wxPayData, "attach", reqDto.Attach)
	SetValue(wxPayData, "fee_type", reqDto.FeeType)
	SetValue(wxPayData, "goods_tag", reqDto.GoodsTag)
	SetValue(wxPayData, "limit_pay", reqDto.LimitPay)
	if len(strings.TrimSpace(reqDto.SpbillCreateIp)) == 0 {
		SetValue(wxPayData, "spbill_create_ip", "8.8.8.8")
	} else {
		SetValue(wxPayData, "spbill_create_ip", reqDto.SpbillCreateIp)
	}
	signStr := base.JoinMapObject(wxPayData.DataAttr)
	SetValue(wxPayData, "sign", sign.MakeMd5Sign(signStr, custDto.Key))
	_, body, err := httpreq.NewPost(URLPAY, []byte(wxPayData.ToXml()),
		&httpreq.Header{ContentType: httpreq.MIMEApplicationXMLCharsetUTF8}, nil)
	if err != nil {
		err = fmt.Errorf("%v:%v", MESSAGE_WECHAT, err)
		return
	}
	result, err = RespParse(body, custDto.Key)
	if err != nil {
		if err.Error() == MESSAGE_PAYING {
			result = map[string]interface{}{"out_trade_no": outTradeNo}
		}
		return
	}
	return
}
func Query(reqDto ReqQueryDto, custDto ReqCustomerDto) (result map[string]interface{}, err error) {
	wxPayData := BuildCommonparam(reqDto.ReqBaseDto)

	SetValue(wxPayData, "transaction_id", reqDto.TransactionId)
	SetValue(wxPayData, "out_trade_no", reqDto.OutTradeNo)

	signStr := base.JoinMapObject(wxPayData.DataAttr)
	SetValue(wxPayData, "sign", sign.MakeMd5Sign(signStr, custDto.Key))
	_, body, err := httpreq.NewPost(URLQUERY, []byte(wxPayData.ToXml()),
		&httpreq.Header{ContentType: httpreq.MIMEApplicationXMLCharsetUTF8}, nil)
	if err != nil {
		return
	}
	result, err = RespParse(body, custDto.Key)
	if err != nil {
		return
	}
	return
}

func Refund(reqDto ReqRefundDto, custDto ReqCustomerDto) (result map[string]interface{}, err error) {
	wxPayData := BuildCommonparam(reqDto.ReqBaseDto)
	//query
	queryDto := ReqQueryDto{ReqBaseDto: reqDto.ReqBaseDto, OutTradeNo: reqDto.OutTradeNo}
	queryResult, err := Query(queryDto, custDto)
	if err != nil {
		err = errors.New("refund failure")
		return
	}
	totalFee, ok := queryResult["total_fee"]
	if !ok {
		err = errors.New("refund failure")
		return
	}

	outRefundNo := reqDto.OutRefundNo
	if len(outRefundNo) == 0 {
		outRefundNo = random.Uuid(PRE_OUTREFUNDNO)
	}
	SetValue(wxPayData, "out_refund_no", outRefundNo)
	SetValue(wxPayData, "device_info", reqDto.DeviceInfo)
	SetValue(wxPayData, "transaction_id", reqDto.TransactionId)
	SetValue(wxPayData, "total_fee", totalFee)
	SetValue(wxPayData, "refund_fee", reqDto.RefundFee)

	SetValue(wxPayData, "refund_fee_type", reqDto.RefundFeeType)
	SetValue(wxPayData, "refund_account", reqDto.RefundAccount)
	SetValue(wxPayData, "out_trade_no", reqDto.OutTradeNo)
	signStr := base.JoinMapObject(wxPayData.DataAttr)
	SetValue(wxPayData, "sign", sign.MakeMd5Sign(signStr, custDto.Key))

	tr, err := httpreq.CertTransport(custDto.CertPathName, custDto.CertPathKey, custDto.RootCa)
	if err != nil {
		err = errors.New("Certificate verification failed")
		return
	}
	_, body, err := httpreq.NewPost(URLREFUND, []byte(wxPayData.ToXml()), &httpreq.Header{ContentType: httpreq.MIMEApplicationXMLCharsetUTF8}, tr)
	if err != nil {
		err = fmt.Errorf("%v:%v", MESSAGE_WECHAT, err)
		return
	}
	result, err = RespParse(body, custDto.Key)
	return
}

func Reverse(reqDto ReqReverseDto, custDto ReqCustomerDto, count int, interval int) (result map[string]interface{}, err error) {
	if count <= 0 {
		err = errors.New("The count of reverse must be greater than 0")
		return
	}
	wxPayData := BuildCommonparam(reqDto.ReqBaseDto)

	SetValue(wxPayData, "transaction_id", reqDto.TransactionId)
	SetValue(wxPayData, "out_trade_no", reqDto.OutTradeNo)

	signStr := base.JoinMapObject(wxPayData.DataAttr)
	SetValue(wxPayData, "sign", sign.MakeMd5Sign(signStr, custDto.Key))
	tr, err := httpreq.CertTransport(custDto.CertPathName, custDto.CertPathKey, custDto.RootCa)
	if err != nil {
		err = errors.New("Certificate verification failed")
		return
	}
	_, body, err := httpreq.NewPost(URLREVERSE, []byte(wxPayData.ToXml()), &httpreq.Header{ContentType: httpreq.MIMEApplicationXMLCharsetUTF8}, tr)
	if err != nil {
		err = fmt.Errorf("%v:%v", MESSAGE_WECHAT, err)
		return
	}
	result, err = RespParse(body, custDto.Key)
	if err != nil {
		return
	}
	recallObj, ok := result["recall"]
	if !ok {
		err = errors.New("reverse failure")
		return
	}

	recall := recallObj.(string)
	if recall == "Y" {
		time.Sleep(time.Duration(interval) * time.Second) //10s
		count = count - 1
		return Reverse(reqDto, custDto, count, interval)
	} else if recall == "N" {
		return
	} else {
		err = errors.New("reverse failure")
	}

	return
}

func RefundQuery(reqDto ReqRefundQueryDto, custDto ReqCustomerDto) (result map[string]interface{}, err error) {
	wxPayData := BuildCommonparam(reqDto.ReqBaseDto)

	SetValue(wxPayData, "transaction_id", reqDto.TransactionId)
	SetValue(wxPayData, "out_trade_no", reqDto.OutTradeNo)
	SetValue(wxPayData, "out_refund_no", reqDto.OutRefundNo)
	SetValue(wxPayData, "refund_id", reqDto.RefundId)
	SetValue(wxPayData, "offset", reqDto.Offset)

	signStr := base.JoinMapObject(wxPayData.DataAttr)
	SetValue(wxPayData, "sign", sign.MakeMd5Sign(signStr, custDto.Key))
	_, body, err := httpreq.NewPost(URLREFUNDQUERY, []byte(wxPayData.ToXml()),
		&httpreq.Header{ContentType: httpreq.MIMEApplicationXMLCharsetUTF8}, nil)
	if err != nil {
		return
	}
	result, err = RespParse(body, custDto.Key)
	if err != nil {
		return
	}
	return
}

func PrePay(reqDto ReqPrePayDto, custDto ReqCustomerDto) (result map[string]interface{}, err error) {
	wxPayData := BuildCommonparam(reqDto.ReqBaseDto)
	if len(reqDto.OutTradeNo) == 0 {
		SetValue(wxPayData, "out_trade_no", random.Uuid(PRE_PREOUTTRADENO))
	} else {
		SetValue(wxPayData, "out_trade_no", reqDto.OutTradeNo)
	}

	if len(reqDto.SpbillCreateIp) == 0 {
		SetValue(wxPayData, "spbill_create_ip", reqDto.SpbillCreateIp)
	} else {
		SetValue(wxPayData, "spbill_create_ip", "8.8.8.8")
	}

	SetValue(wxPayData, "sign_type", reqDto.SignType)
	SetValue(wxPayData, "body", reqDto.Body)
	SetValue(wxPayData, "detail", reqDto.Detail)
	SetValue(wxPayData, "attach", reqDto.Attach)
	SetValue(wxPayData, "device_info", reqDto.DeviceInfo)
	SetValue(wxPayData, "fee_type", reqDto.FeeType)

	SetValue(wxPayData, "total_fee", reqDto.TotalFee)

	SetValue(wxPayData, "time_start", base.GetDateFormat(base.GetChinaTime(time.Now()), 121))
	SetValue(wxPayData, "time_expire", base.GetDateFormat(base.GetChinaTime(time.Now().Add(time.Minute*10)), 121))
	SetValue(wxPayData, "goods_tag", reqDto.GoodsTag)

	if len(reqDto.NotifyUrl) != 0 {
		SetValue(wxPayData, "notify_url", reqDto.NotifyUrl)
	} else {
		SetValue(wxPayData, "notify_url", custDto.UnifiedNotifyUrl)
	}
	SetValue(wxPayData, "trade_type", reqDto.TradeType)
	SetValue(wxPayData, "product_id", reqDto.ProductId)
	SetValue(wxPayData, "limit_pay", reqDto.LimitPay)
	SetValue(wxPayData, "openid", reqDto.OpenId)
	SetValue(wxPayData, "sub_openid", reqDto.SubOpenId)

	SetValue(wxPayData, "scene_info", reqDto.SceneInfo)
	signStr := base.JoinMapObject(wxPayData.DataAttr)
	SetValue(wxPayData, "sign", sign.MakeMd5Sign(signStr, custDto.Key))
	_, body, err := httpreq.NewPost(URLPREPAY, []byte(wxPayData.ToXml()), &httpreq.Header{ContentType: httpreq.MIMEApplicationXMLCharsetUTF8}, nil)
	if err != nil {
		err = fmt.Errorf("%v:%v", MESSAGE_WECHAT, err)
		return
	}
	result, err = RespParse(body, custDto.Key)
	return
}

func NotifyForPrePay(xmlBody string) (result interface{}, err error) {
	dataObj := data.New()
	err = dataObj.FromXml(xmlBody)
	if err != nil {
		err = fmt.Errorf("%v:%v", MESSAGE_WECHAT, err)
		return
	}

	if !dataObj.IsSet("attach") {
		err = errors.New("notify_url is required in attach,but attach is empty")
		return
	}
	var attachObj struct {
		NotifyUrl string `json:"NotifyUrl"`
	}
	err = json.Unmarshal([]byte(dataObj.DataAttr["attach"].(string)), attachObj)
	if err != nil {
		err = errors.New("attach's format is expected to json")
		return
	}
	if !dataObj.IsSet("notify_url") {
		err = errors.New("notify_url is missing in attach")
		return
	}
	_, err = httpreq.POST("", attachObj.NotifyUrl, dataObj.DataAttr, nil)
	if err != nil {
		return
	}
	type SuccessResult struct {
		XMLName    xml.Name `xml:"xml"`
		ReturnCode string   `xml:"return_code"`
		ReturnMsg  string   `xml:"return_msg"`
	}
	successResult := &SuccessResult{
		ReturnCode: "SUCCESS",
		ReturnMsg:  "OK",
	}
	result = successResult
	return

}
