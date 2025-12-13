package handlers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/hoshinonyaruko/gensokyo/callapi"
	"github.com/hoshinonyaruko/gensokyo/config"
	"github.com/hoshinonyaruko/gensokyo/idmap"
	"github.com/hoshinonyaruko/gensokyo/mylog"
	"github.com/hoshinonyaruko/gensokyo/url"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/errs"
	"mvdan.cc/xurls" //xurls是一个从文本提取url的库 适用于多种场景
)

var BotID string
var AppID string

// ErrorResponse QQ API 错误响应结构
type ErrorResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
	ErrCode int    `json:"err_code"`
	TraceID string `json:"trace_id"`
}

// 定义响应结构体
type ServerResponse struct {
	Data struct {
		MessageID int `json:"message_id"`
	} `json:"data"`
	Message   string      `json:"message"`
	RetCode   int         `json:"retcode"`
	Status    string      `json:"status"`
	Echo      interface{} `json:"echo,omitempty"`
	RequestID interface{} `json:"request_id,omitempty"`
}

// 发送成功回执 todo 返回可互转的messageid
func SendResponse(client callapi.Client, err error, message *callapi.ActionMessage) error {
	// 设置响应值
	response := ServerResponse{}
	response.Data.MessageID = 0 // todo 实现messageid转换
	// 根据配置决定返回字段名
	if config.GetUseRequestID() {
		response.RequestID = callapi.GetActionEchoKey(*message)
	} else {
		response.Echo = message.Echo
	}
	if err != nil {
		// 过滤敏感信息后再返回错误消息
		response.Message = sanitizeErrorMessage(err)

		// 检查是否是真实的错误（如富媒体文件下载失败）而不是审核中的假错误
		if isRealFailure(err) {
			// 真实失败：返回失败状态
			response.RetCode = -1
			response.Status = "failed"
			mylog.Printf("检测到真实错误，返回失败状态: %v", err)
		} else {
			// 审核中或其他可忽略错误：返回成功状态（保持原有行为）
			response.RetCode = 0 // 官方api审核异步的 审核中默认返回失败,但其实信息发送成功了
			response.Status = "ok"
		}
	} else {
		response.Message = ""
		response.RetCode = 0
		response.Status = "ok"
	}

	// 转化为map并发送
	outputMap := structToMap(response)

	mylog.Printf("准备发送回执: %+v", outputMap)
	sendErr := client.SendMessage(outputMap)
	if sendErr != nil {
		mylog.Printf("发送回执失败: %v", sendErr)
		return sendErr
	}

	mylog.Printf("发送成功回执完成")
	return nil
}

// sanitizeErrorMessage 过滤错误消息中的敏感信息，只保留 QQ Bot 官方错误信息
func sanitizeErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	// 尝试将错误转换为 SDK 的 Err 类型
	sdkErr := errs.Error(err)
	errText := sdkErr.Text()

	var errResp ErrorResponse
	if jsonErr := json.Unmarshal([]byte(errText), &errResp); jsonErr != nil {
		// 如果无法解析为 JSON，返回通用错误消息（不暴露具体细节）
		return fmt.Sprintf("code:%d, 请求失败", sdkErr.Code())
	}

	// 只返回官方 API 的错误信息，不包含可能的敏感信息（如 URL、IP 等）
	// 返回格式与官方错误一致
	return fmt.Sprintf("code:%d, message:%s, err_code:%d",
		errResp.Code, errResp.Message, errResp.ErrCode)
}

// isRealFailure 判断是否是真实的失败（而非审核中的假失败）
func isRealFailure(err error) bool {
	// 如果错误为 nil，返回 false
	if err == nil {
		return false
	}

	// 将错误转换为 SDK 的 Err 类型（errs.Error 永远不会返回 nil）
	sdkErr := errs.Error(err)

	// 获取错误文本并尝试解析为 JSON
	errText := sdkErr.Text()

	var errResp ErrorResponse
	if jsonErr := json.Unmarshal([]byte(errText), &errResp); jsonErr != nil {
		// 如果无法解析为 JSON，可能是其他类型的错误
		// 保守处理：如果 HTTP 状态码是 4xx 或 5xx，认为是真实错误
		if sdkErr.Code() >= 400 {
			return true
		}
		return false
	}

	// 检查特定的错误码
	// 850026 - 富媒体文件下载失败
	// 40034001 - 相关的错误码
	// 304023 - 消息审核不通过（这个应该视为审核失败，保持原有行为返回ok）
	switch errResp.Code {
	case 850026: // 富媒体文件下载失败
		return true
	case 40034001: // 相关错误
		return true
	case 304023: // 消息审核不通过 - 保持原有行为（返回ok）
		return false
	default:
		// 对于未知的错误码，保守处理：
		// HTTP 4xx/5xx 状态码通常表示真实错误（如参数错误、服务器错误等）
		// 只有明确是审核类错误才返回 false，其他 4xx/5xx 都视为真实错误
		if sdkErr.Code() >= 400 {
			return true
		}
		return false
	}
}

// 信息处理函数
func parseMessageContent(paramsMessage callapi.ParamsContent) (string, map[string][]string) {
	messageText := ""

	switch message := paramsMessage.Message.(type) {
	case string:
		mylog.Printf("params.message is a string\n")
		messageText = message
	case []interface{}:
		//多个映射组成的切片
		mylog.Printf("params.message is a slice (segment_type_koishi)\n")
		for _, segment := range message {
			segmentMap, ok := segment.(map[string]interface{})
			if !ok {
				continue
			}

			segmentType, ok := segmentMap["type"].(string)
			if !ok {
				continue
			}

			segmentContent := ""
			switch segmentType {
			case "text":
				segmentContent, _ = segmentMap["data"].(map[string]interface{})["text"].(string)
			case "image":
				fileContent, _ := segmentMap["data"].(map[string]interface{})["file"].(string)
				segmentContent = "[CQ:image,file=" + fileContent + "]"
			case "voice":
				fileContent, _ := segmentMap["data"].(map[string]interface{})["file"].(string)
				segmentContent = "[CQ:record,file=" + fileContent + "]"
			case "at":
				qqNumber, _ := segmentMap["data"].(map[string]interface{})["qq"].(string)
				segmentContent = "[CQ:at,qq=" + qqNumber + "]"
			}

			messageText += segmentContent
		}
	case map[string]interface{}:
		//单个映射
		mylog.Printf("params.message is a map (segment_type_trss)\n")
		messageType, _ := message["type"].(string)
		switch messageType {
		case "text":
			messageText, _ = message["data"].(map[string]interface{})["text"].(string)
		case "image":
			fileContent, _ := message["data"].(map[string]interface{})["file"].(string)
			messageText = "[CQ:image,file=" + fileContent + "]"
		case "voice":
			fileContent, _ := message["data"].(map[string]interface{})["file"].(string)
			messageText = "[CQ:record,file=" + fileContent + "]"
		case "at":
			qqNumber, _ := message["data"].(map[string]interface{})["qq"].(string)
			messageText = "[CQ:at,qq=" + qqNumber + "]"
		}
	default:
		mylog.Println("Unsupported message format: params.message field is not a string, map or slice")
	}

	// 正则表达式部分
	var localImagePattern *regexp.Regexp

	if runtime.GOOS == "windows" {
		localImagePattern = regexp.MustCompile(`\[CQ:image,file=file:///([^\]]+?)\]`)
	} else {
		localImagePattern = regexp.MustCompile(`\[CQ:image,file=file://([^\]]+?)\]`)
	}

	urlImagePattern := regexp.MustCompile(`\[CQ:image,file=https?://(.+)\]`)
	base64ImagePattern := regexp.MustCompile(`\[CQ:image,file=base64://(.+)\]`)
	base64RecordPattern := regexp.MustCompile(`\[CQ:record,file=base64://(.+)\]`)

	patterns := []struct {
		key     string
		pattern *regexp.Regexp
	}{
		{"local_image", localImagePattern},
		{"url_image", urlImagePattern},
		{"base64_image", base64ImagePattern},
		{"base64_record", base64RecordPattern},
	}

	foundItems := make(map[string][]string)
	for _, pattern := range patterns {
		matches := pattern.pattern.FindAllStringSubmatch(messageText, -1)
		for _, match := range matches {
			if len(match) > 1 {
				foundItems[pattern.key] = append(foundItems[pattern.key], match[1])
				messageText = pattern.pattern.ReplaceAllString(messageText, "")
			}
		}
	}
	//处理at
	messageText = transformMessageText(messageText)

	return messageText, foundItems
}

// at处理和链接处理
func transformMessageText(messageText string) string {
	// 首先，将AppID替换为BotID
	messageText = strings.ReplaceAll(messageText, AppID, BotID)

	// 去除所有[CQ:reply,id=数字] todo 更好的处理办法
	replyRE := regexp.MustCompile(`\[CQ:reply,id=\d+\]`)
	messageText = replyRE.ReplaceAllString(messageText, "")

	// 使用正则表达式来查找所有[CQ:at,qq=数字]的模式
	re := regexp.MustCompile(`\[CQ:at,qq=(\d+)\]`)
	messageText = re.ReplaceAllStringFunc(messageText, func(m string) string {
		submatches := re.FindStringSubmatch(m)
		if len(submatches) > 1 {
			realUserID, err := idmap.RetrieveRowByIDv2(submatches[1])
			if err != nil {
				// 如果出错，也替换成相应的格式，但使用原始QQ号
				mylog.Printf("Error retrieving user ID: %v", err)
				return "<@!" + submatches[1] + ">"
			}
			return "<@!" + realUserID + ">"
		}
		return m
	})

	// 使用xurls来查找和替换所有的URL
	if config.GetLotusValue() {
		// 连接到另一个gensokyo
		messageText = xurls.Relaxed.ReplaceAllStringFunc(messageText, func(originalURL string) string {
			shortURL := url.GenerateShortURL(originalURL)
			return shortURL
		})
	} else {
		// Lotus is false, prepend the base URL to the shortURL
		messageText = xurls.Relaxed.ReplaceAllStringFunc(messageText, func(originalURL string) string {
			shortURL := url.GenerateShortURL(originalURL)
			// 使用getBaseURL函数来获取baseUrl并与shortURL组合
			return url.GetBaseURL() + "/url/" + shortURL
		})
	}
	return messageText
}

// 处理at和其他定形文到onebotv11格式(cq码)
func RevertTransformedText(data interface{}) string {
	var msg *dto.Message
	switch v := data.(type) {
	case *dto.WSGroupATMessageData:
		msg = (*dto.Message)(v)
	case *dto.WSATMessageData:
		msg = (*dto.Message)(v)
	case *dto.WSMessageData:
		msg = (*dto.Message)(v)
	case *dto.WSDirectMessageData:
		msg = (*dto.Message)(v)
	case *dto.WSC2CMessageData:
		msg = (*dto.Message)(v)
	default:
		return ""
	}
	//处理前 先去前后空
	messageText := strings.TrimSpace(msg.Content)

	// 将messageText里的BotID替换成AppID
	messageText = strings.ReplaceAll(messageText, BotID, AppID)

	// 使用正则表达式来查找所有<@!数字>的模式
	re := regexp.MustCompile(`<@!(\d+)>`)
	// 使用正则表达式来替换找到的模式为[CQ:at,qq=用户ID]
	messageText = re.ReplaceAllStringFunc(messageText, func(m string) string {
		submatches := re.FindStringSubmatch(m)
		if len(submatches) > 1 {
			userID := submatches[1]
			// 检查是否是 BotID，如果是则直接返回，不进行映射,或根据用户需求移除
			if userID == AppID {
				if config.GetRemoveAt() {
					return ""
				} else {
					return "[CQ:at,qq=" + AppID + "]"
				}
			}

			// 不是 BotID，进行正常映射
			userID64, err := idmap.StoreIDv2(userID)
			if err != nil {
				//如果储存失败(数据库损坏)返回原始值
				mylog.Printf("Error storing ID: %v", err)
				return "[CQ:at,qq=" + userID + "]"
			}
			// 类型转换
			userIDStr := strconv.FormatInt(userID64, 10)
			// 经过转换的cq码
			return "[CQ:at,qq=" + userIDStr + "]"
		}
		return m
	})

	// 检查是否需要移除前缀
	if config.GetRemovePrefixValue() {
		// 移除消息内容中第一次出现的 "/"
		if idx := strings.Index(messageText, "/"); idx != -1 {
			messageText = messageText[:idx] + messageText[idx+1:]
		}
	}

	// 处理图片附件
	for _, attachment := range msg.Attachments {
		if strings.HasPrefix(attachment.ContentType, "image/") {
			// 获取文件的后缀名
			ext := filepath.Ext(attachment.FileName)
			md5name := strings.TrimSuffix(attachment.FileName, ext)
			imageCQ := "[CQ:image,file=" + md5name + ".image,subType=0,url=" + "http://" + attachment.URL + "]"
			messageText += imageCQ
		}
	}

	//如果移除了前部at,信息就会以空格开头,因为只移去了最前面的at,但at后紧跟随一个空格
	if config.GetRemoveAt() {
		//再次去前后空
		messageText = strings.TrimSpace(messageText)
	}

	return messageText
}

// 将收到的data.content转换为message segment todo,群场景不支持受图片,频道场景的图片可以拼一下
func ConvertToSegmentedMessage(data interface{}) []map[string]interface{} {
	// 强制类型转换，获取Message结构
	var msg *dto.Message
	switch v := data.(type) {
	case *dto.WSGroupATMessageData:
		msg = (*dto.Message)(v)
	case *dto.WSATMessageData:
		msg = (*dto.Message)(v)
	case *dto.WSMessageData:
		msg = (*dto.Message)(v)
	case *dto.WSDirectMessageData:
		msg = (*dto.Message)(v)
	case *dto.WSC2CMessageData:
		msg = (*dto.Message)(v)
	default:
		return nil
	}

	var messageSegments []map[string]interface{}

	// 处理Attachments字段来构建图片消息
	for _, attachment := range msg.Attachments {
		imageFileMD5 := attachment.FileName
		for _, ext := range []string{"{", "}", ".png", ".jpg", ".gif", "-"} {
			imageFileMD5 = strings.ReplaceAll(imageFileMD5, ext, "")
		}
		imageSegment := map[string]interface{}{
			"type": "image",
			"data": map[string]interface{}{
				"file":    imageFileMD5 + ".image",
				"subType": "0",
				"url":     attachment.URL,
			},
		}
		messageSegments = append(messageSegments, imageSegment)

		// 在msg.Content中替换旧的图片链接
		newImagePattern := "[CQ:image,file=" + attachment.URL + "]"
		msg.Content = msg.Content + newImagePattern
	}

	// 使用正则表达式查找所有的[@数字]格式
	r := regexp.MustCompile(`<@!(\d+)>`)
	atMatches := r.FindAllStringSubmatch(msg.Content, -1)

	for _, match := range atMatches {
		// 构建at部分的映射并加入到messageSegments
		atSegment := map[string]interface{}{
			"type": "at",
			"data": map[string]interface{}{
				"qq": match[1],
			},
		}
		messageSegments = append(messageSegments, atSegment)

		// 从原始内容中移除at部分
		msg.Content = strings.Replace(msg.Content, match[0], "", 1)
	}

	// 如果还有其他内容，那么这些内容被视为文本部分
	if msg.Content != "" {
		textSegment := map[string]interface{}{
			"type": "text",
			"data": map[string]interface{}{
				"text": msg.Content,
			},
		}
		messageSegments = append(messageSegments, textSegment)
	}

	return messageSegments
}
