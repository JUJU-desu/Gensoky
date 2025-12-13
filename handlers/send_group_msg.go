package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/hoshinonyaruko/gensokyo/callapi"
	"github.com/hoshinonyaruko/gensokyo/config"
	"github.com/hoshinonyaruko/gensokyo/echo"
	"github.com/hoshinonyaruko/gensokyo/idmap"
	"github.com/hoshinonyaruko/gensokyo/images"
	"github.com/hoshinonyaruko/gensokyo/mylog"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/openapi"
)

func init() {
	callapi.RegisterHandler("send_group_msg", handleSendGroupMsg)
}

func handleSendGroupMsg(client callapi.Client, api openapi.OpenAPI, apiv2 openapi.OpenAPI, message callapi.ActionMessage) {
	// 使用 message.Echo 作为key来获取消息类型
	var msgType string
	echoVal := callapi.GetActionEchoKey(message)
	if echoStr, ok := resolveEchoToString(echoVal); ok {
		// 当 message.Echo 或 request_id 是字符串类型时执行此块
		msgType = echo.GetMsgTypeByKey(echoStr)
	}

	//如果获取不到 就用user_id获取信息类型
	if msgType == "" {
		msgType = GetMessageTypeByUserid(config.GetAppIDStr(), message.Params.UserID)
	}

	//如果获取不到 就用group_id获取信息类型
	if msgType == "" {
		msgType = GetMessageTypeByGroupid(config.GetAppIDStr(), message.Params.GroupID)
	}

	switch msgType {
	case "group":
		// 解析消息内容
		messageText, foundItems := parseMessageContent(message.Params)

		// 使用 echo 获取消息ID
		var messageID string
		if echoStr, ok := resolveEchoToString(echoVal); ok {
			messageID = echo.GetMsgIDByKey(echoStr)
			mylog.Println("echo取群组发信息对应的message_id:", messageID)
		}
		//通过bolt数据库还原真实的GroupID
		originalGroupID, err := idmap.RetrieveRowByIDv2(message.Params.GroupID.(string))
		if err != nil {
			mylog.Printf("Error retrieving original GroupID: %v", err)
			return
		}
		message.Params.GroupID = originalGroupID
		mylog.Println("群组发信息messageText:", messageText)

		// 第一步：尝试从echo获取MessageID，然后反查UserID
		var realUserID int64
		var realMsgID string

		if messageID != "" {
			// 从MessageID反查UserID
			realUserID = echo.GetUserIDByMsgID(messageID)
			if realUserID > 0 {
				mylog.Println("通过echo的MessageID反查到UserID:", messageID, "->", realUserID)
				realMsgID = messageID
			}
		}

		// 第二步：如果echo没有，尝试从Params.UserID获取
		if realUserID == 0 && message.Params.UserID != nil {
			switch v := message.Params.UserID.(type) {
			case string:
				if v != "" && v != "0" {
					if uid, err := strconv.ParseInt(v, 10, 64); err == nil {
						realUserID = uid
					}
				}
			case int:
				realUserID = int64(v)
			case int64:
				realUserID = v
			case float64:
				realUserID = int64(v)
			}
			if realUserID > 0 {
				mylog.Println("从Params.UserID获取到UserID:", realUserID)
			}
		}

		// 第三步：如果还没有UserID，从GroupID的待处理消息队列获取（解决并发问题）
		if realUserID == 0 {
			// 如果启用request_id，则跳过FIFO队列获取，使用request_id映射获取UserID
			if config.GetUseRequestID() {
				mylog.Printf("use_requestid已启用，跳过队列以减少开销")
			} else {
				//还原真实GroupID的int64值
				groupIDInt64, err := idmap.StoreIDv2(message.Params.GroupID.(string))
				if err == nil {
					// 使用FIFO队列获取最早的待处理消息（解决并发时的错误路由）
					queueUserID, queueMsgID := echo.PopGroupPendingMessage(groupIDInt64)
					if queueUserID > 0 && queueMsgID != "" {
						realUserID = queueUserID
						realMsgID = queueMsgID
						mylog.Printf("从待处理队列获取: GroupID[%d] -> UserID[%d], MsgID[%s]", groupIDInt64, realUserID, realMsgID)
					} else {
						// 如果队列为空，降级到旧方案（向后兼容）
						realUserID = echo.GetGroupLatestUser(groupIDInt64)
						if realUserID > 0 {
							mylog.Printf("队列为空，降级方案: 通过GroupID[%d]查询到最近的UserID: %d", groupIDInt64, realUserID)
						}
					}
				}
			}
		}

		// 第四步：如果有UserID但还没有MsgID，用UserID来获取准确的原始MessageID
		if realUserID > 0 && realMsgID == "" {
			realMsgID = GetMessageIDByUseridOrGroupid(config.GetAppIDStr(), realUserID)
			mylog.Println("使用UserID", realUserID, "查询到原始message_id:", realMsgID)
		}

		// 第五步：最终兜底 - 如果所有方法都失败了
		if realMsgID == "" {
			mylog.Printf("警告：无法获取message_id，将尝试发送但可能失败（无被动回复权限）")
		}

		// 使用查询到的messageID
		messageID = realMsgID
		mylog.Printf("最终使用的message_id: [%s]", messageID)

		// 优先发送文本信息
		if messageText != "" {
			groupReply := generateGroupMessage(messageID, nil, messageText)

			// 进行类型断言
			groupMessage, ok := groupReply.(*dto.MessageToCreate)
			if !ok {
				mylog.Println("Error: Expected MessageToCreate type.")
				return // 或其他错误处理
			}

			groupMessage.Timestamp = time.Now().Unix() // 设置时间戳
			//重新为err赋值
			_, err = apiv2.PostGroupMessage(context.TODO(), message.Params.GroupID.(string), groupMessage)
			if err != nil {
				mylog.Printf("发送文本群组信息失败: %v", err)
				// 如果是真实错误，尝试发送错误提示（可能也会失败，但至少尝试一次）
				if isRealFailure(err) {
					errorMsg := &dto.MessageToCreate{
						Content: fmt.Sprintf("消息发送失败: %v", err),
						MsgID:   messageID,
						MsgType: 0,
					}
					apiv2.PostGroupMessage(context.TODO(), message.Params.GroupID.(string), errorMsg)
				}
			}
			//发送成功回执
			SendResponse(client, err, &message)
		}

		// 遍历foundItems并发送每种信息（两步法发送图片）
		var lastErr error
		for key, urls := range foundItems {
			mylog.Printf("准备发送 %s，URL: %v", key, urls)

			var singleItem = make(map[string][]string)
			singleItem[key] = urls
			groupReply := generateGroupMessage(messageID, singleItem, "")

			// 进行类型断言
			richMediaMessage, ok := groupReply.(*dto.RichMediaMessage)
			if !ok {
				mylog.Printf("Error: Expected RichMediaMessage type for key %s.", key)
				continue
			}

			// 步骤1: 上传图片获取 file_info（不直接发送，避免占用主动消息频次）
			richMediaMessage.SrvSendMsg = false // 关键：仅上传，不发送
			mylog.DebugPrintf("步骤1: 上传富媒体文件 - URL: %s", richMediaMessage.URL)

			// 使用 Transport 方法直接调用上传接口
			uploadData, err := apiv2.Transport(context.TODO(), "POST",
				fmt.Sprintf("https://api.sgroup.qq.com/v2/groups/%s/files", message.Params.GroupID.(string)),
				richMediaMessage)
			if err != nil {
				mylog.Printf("上传富媒体失败: %v", err)
				lastErr = err

				// 向QQ群发送错误提示消息
				errorMsg := &dto.MessageToCreate{
					Content: fmt.Sprintf("图片发送失败: %v", err),
					MsgID:   messageID,
					MsgType: 0, // 文本消息
				}
				apiv2.PostGroupMessage(context.TODO(), message.Params.GroupID.(string), errorMsg)

				// 向WebSocket客户端返回错误响应
				SendResponse(client, err, &message)
				return
			}

			mylog.Printf("上传成功，响应: %s", string(uploadData))

			// 解析上传响应获取 file_info
			var uploadResp dto.RichMediaResponse
			if err := json.Unmarshal(uploadData, &uploadResp); err != nil {
				mylog.Printf("解析上传响应失败: %v", err)
				lastErr = err

				// 向QQ群发送错误提示消息
				errorMsg := &dto.MessageToCreate{
					Content: fmt.Sprintf("图片发送失败: %v", err),
					MsgID:   messageID,
					MsgType: 0, // 文本消息
				}
				apiv2.PostGroupMessage(context.TODO(), message.Params.GroupID.(string), errorMsg)

				// 向WebSocket客户端返回错误响应
				SendResponse(client, err, &message)
				return
			}

			mylog.Printf("获取到 file_info: %s, TTL: %d 秒", uploadResp.FileInfo, uploadResp.TTL)

			// 步骤2: 使用标准消息接口发送（被动回复模式）
			mediaMessage := &dto.MessageToCreate{
				Content: " ",       // 官方要求：msg_type=7 时需要填空格
				MsgType: 7,         // 7 = 富媒体消息
				MsgID:   messageID, // 被动回复的关键
				Media: &dto.Media{
					FileInfo: uploadResp.FileInfo,
				},
			}

			mylog.DebugPrintf("步骤2: 发送富媒体消息 - GroupID: %s, MsgID: %s", message.Params.GroupID.(string), messageID)
			_, err = apiv2.PostGroupMessage(context.TODO(), message.Params.GroupID.(string), mediaMessage)
			if err != nil {
				mylog.Printf("发送 %s 信息失败: %v", key, err)
				lastErr = err
			} else {
				mylog.Printf("发送 %s 信息成功（图片已发送）", key)
			}
		}
		// 所有媒体项处理完毕后，发送最终响应（如果有错误，返回最后一个错误）
		SendResponse(client, lastErr, &message)
	case "guild":
		//用GroupID给ChannelID赋值,因为我们是把频道虚拟成了群
		message.Params.ChannelID = message.Params.GroupID.(string)
		// 使用RetrieveRowByIDv2还原真实的ChannelID
		RChannelID, err := idmap.RetrieveRowByIDv2(message.Params.ChannelID)
		if err != nil {
			mylog.Printf("error retrieving real UserID: %v", err)
		}
		message.Params.ChannelID = RChannelID
		//这一句是group_private的逻辑,发频道信息用的是channelid
		//message.Params.GroupID = value
		handleSendGuildChannelMsg(client, api, apiv2, message)
	case "guild_private":
		//用group_id还原出channelid 这是虚拟成群的私聊信息
		message.Params.ChannelID = message.Params.GroupID.(string)
		// 使用RetrieveRowByIDv2还原真实的ChannelID
		RChannelID, err := idmap.RetrieveRowByIDv2(message.Params.ChannelID)
		if err != nil {
			mylog.Printf("error retrieving real ChannelID: %v", err)
		}
		//读取ini 通过ChannelID取回之前储存的guild_id
		value, err := idmap.ReadConfigv2(RChannelID, "guild_id")
		if err != nil {
			mylog.Printf("Error reading config: %v", err)
			return
		}
		handleSendGuildChannelPrivateMsg(client, api, apiv2, message, &value, &message.Params.ChannelID)
	case "group_private":
		//用userid还原出openid 这是虚拟成群的群聊私聊信息
		message.Params.UserID = message.Params.GroupID.(string)
		handleSendPrivateMsg(client, api, apiv2, message)
	default:
		mylog.Printf("Unknown message type: %s", msgType)
	}
}

// 不支持base64
func generateGroupMessage(id string, foundItems map[string][]string, messageText string) interface{} {
	if imageURLs, ok := foundItems["local_image"]; ok && len(imageURLs) > 0 {
		// 从本地路径读取图片
		imageData, err := os.ReadFile(imageURLs[0])
		if err != nil {
			// 读入文件失败
			mylog.Printf("Error reading the image from path %s: %v", imageURLs[0], err)
			// 返回文本信息，提示图片文件不存在
			return &dto.MessageToCreate{
				Content: "错误: 图片文件不存在",
				MsgID:   id,
				MsgType: 0, // 默认文本类型
			}
		}
		// 首先压缩图片 默认不压缩
		compressedData, err := images.CompressSingleImage(imageData)
		if err != nil {
			mylog.Printf("Error compressing image: %v", err)
			return &dto.MessageToCreate{
				Content: "错误: 压缩图片失败",
				MsgID:   id,
				MsgType: 0, // 默认文本类型
			}
		}

		// base64编码
		base64Encoded := base64.StdEncoding.EncodeToString(compressedData)

		// 上传base64编码的图片并获取其URL
		imageURL, err := images.UploadBase64ImageToServer(base64Encoded)
		if err != nil {
			mylog.Printf("Error uploading base64 encoded image: %v", err)
			// 如果上传失败，也返回文本信息，提示上传失败
			return &dto.MessageToCreate{
				Content: "错误: 上传图片失败",
				MsgID:   id,
				MsgType: 0, // 默认文本类型
			}
		}

		// 创建RichMediaMessage并返回，当作URL图片处理
		return &dto.RichMediaMessage{
			EventID:    id,
			MsgID:      id,
			FileType:   1, // 1代表图片
			URL:        imageURL,
			Content:    " ",  // 官方要求：msg_type=7 时需要填空格
			SrvSendMsg: true, // 直接发送（被动回复模式）
		}
	} else if imageURLs, ok := foundItems["url_image"]; ok && len(imageURLs) > 0 {
		// 发链接图片
		// 根据官方文档，当 srv_send_msg=true 时会占用主动消息频次
		// 必须包含 msg_id 或 event_id 才能作为被动回复
		return &dto.RichMediaMessage{
			EventID:    id,                       // 被动回复的事件ID
			MsgID:      id,                       // 被动回复的消息ID（与EventID作用相同）
			FileType:   1,                        // 1代表图片
			URL:        "http://" + imageURLs[0], //url在base64时候被截断了,在这里补全
			Content:    " ",                      // 官方要求：msg_type=7 时需要填空格
			SrvSendMsg: true,                     // 直接发送消息
		}
	} else if voiceURLs, ok := foundItems["base64_record"]; ok && len(voiceURLs) > 0 {
		// 目前不支持发语音 todo 适配base64 slik
	} else if base64_image, ok := foundItems["base64_image"]; ok && len(base64_image) > 0 {
		// todo 适配base64图片
		//因为QQ群没有 form方式上传,所以在gensokyo内置了图床,需公网,或以lotus方式连接位于公网的gensokyo
		//要正确的开放对应的端口和设置正确的ip地址在config,这对于一般用户是有一些难度的
		if base64Image, ok := foundItems["base64_image"]; ok && len(base64Image) > 0 {
			// 解码base64图片数据
			fileImageData, err := base64.StdEncoding.DecodeString(base64Image[0])
			if err != nil {
				mylog.Printf("failed to decode base64 image: %v", err)
				return nil
			}
			// 首先压缩图片 默认不压缩
			compressedData, err := images.CompressSingleImage(fileImageData)
			if err != nil {
				mylog.Printf("Error compressing image: %v", err)
				return &dto.MessageToCreate{
					Content: "错误: 压缩图片失败",
					MsgID:   id,
					MsgType: 0, // 默认文本类型
				}
			}
			// 将解码的图片数据转换回base64格式并上传
			imageURL, err := images.UploadBase64ImageToServer(base64.StdEncoding.EncodeToString(compressedData))
			if err != nil {
				mylog.Printf("failed to upload base64 image: %v", err)
				return nil
			}
			// 创建RichMediaMessage并返回
			return &dto.RichMediaMessage{
				EventID:    id,
				MsgID:      id,
				FileType:   1, // 1代表图片
				URL:        imageURL,
				Content:    " ",  // 官方要求：msg_type=7 时需要填空格
				SrvSendMsg: true, // 直接发送（被动回复模式）
			}
		}
	} else {
		// 返回文本信息
		return &dto.MessageToCreate{
			Content: messageText,
			MsgID:   id,
			MsgType: 0, // 默认文本类型
		}
	}
	return nil
}

// 通过user_id获取类型
func GetMessageTypeByUserid(appID string, userID interface{}) string {
	// 从appID和userID生成key
	var userIDStr string
	switch u := userID.(type) {
	case int:
		userIDStr = strconv.Itoa(u)
	case int64:
		userIDStr = strconv.FormatInt(u, 10)
	case float64:
		userIDStr = strconv.FormatFloat(u, 'f', 0, 64)
	case string:
		userIDStr = u
	default:
		// 可能需要处理其他类型或报错
		return ""
	}

	key := appID + "_" + userIDStr
	return echo.GetMsgTypeByKey(key)
}

// 通过group_id获取类型
func GetMessageTypeByGroupid(appID string, GroupID interface{}) string {
	// 从appID和userID生成key
	var GroupIDStr string
	switch u := GroupID.(type) {
	case int:
		GroupIDStr = strconv.Itoa(u)
	case int64:
		GroupIDStr = strconv.FormatInt(u, 10)
	case string:
		GroupIDStr = u
	default:
		// 可能需要处理其他类型或报错
		return ""
	}

	key := appID + "_" + GroupIDStr
	return echo.GetMsgTypeByKey(key)
}
