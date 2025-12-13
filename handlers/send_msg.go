package handlers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hoshinonyaruko/gensokyo/callapi"
	"github.com/hoshinonyaruko/gensokyo/config"
	"github.com/hoshinonyaruko/gensokyo/echo"
	"github.com/hoshinonyaruko/gensokyo/idmap"
	"github.com/hoshinonyaruko/gensokyo/mylog"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/openapi"
)

func init() {
	callapi.RegisterHandler("send_msg", handleSendMsg)
}

func handleSendMsg(client callapi.Client, api openapi.OpenAPI, apiv2 openapi.OpenAPI, message callapi.ActionMessage) {
	// 使用 message.Echo 作为key来获取消息类型
	var msgType string
	echoVal := callapi.GetActionEchoKey(message)
	if echoStr, ok := resolveEchoToString(echoVal); ok {
		// 当 message.Echo 或 request_id 是字符串类型时执行此块
		msgType = echo.GetMsgTypeByKey(echoStr)
	}

	//如果获取不到 就用group_id获取信息类型
	if msgType == "" {
		appID := config.GetAppIDStr()
		groupID := message.Params.GroupID
		mylog.Printf("appID: %s, GroupID: %v\n", appID, groupID)

		msgType = GetMessageTypeByGroupid(appID, groupID)
		mylog.Printf("msgType: %s\n", msgType)
	}

	// 如果获取不到 就用 user_id 获取信息类型（已弃用：使用 request_id 优先）
	// 保留严格后备逻辑以兼容旧客户端
	if msgType == "" && message.Params.UserID != nil {
		msgType = GetMessageTypeByUserid(config.GetAppIDStr(), message.Params.UserID)
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
		mylog.Println("群组发信息messageText:", messageText)
		//通过bolt数据库还原真实的GroupID
		originalGroupID, err := idmap.RetrieveRowByIDv2(message.Params.GroupID.(string))
		if err != nil {
			mylog.Printf("Error retrieving original GroupID: %v", err)
			return
		}
		message.Params.GroupID = originalGroupID
		// 如果messageID为空，通过函数获取
		if messageID == "" {
			messageID = GetMessageIDByUseridOrGroupid(config.GetAppIDStr(), message.Params.GroupID)
			mylog.Println("通过GetMessageIDByUserid函数获取的message_id:", messageID)
		}
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
			_, err = apiv2.PostGroupMessage(context.TODO(), message.Params.GroupID.(string), groupMessage)
			if err != nil {
				mylog.Printf("发送文本群组信息失败: %v", err)
				// 如果是真实错误，尝试发送错误提示
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

		// 遍历foundItems并发送每种信息
		for key, urls := range foundItems {
			var singleItem = make(map[string][]string)
			singleItem[key] = urls

			groupReply := generateGroupMessage(messageID, singleItem, "")

			// 进行类型断言
			richMediaMessage, ok := groupReply.(*dto.RichMediaMessage)
			if !ok {
				mylog.Printf("Error: Expected RichMediaMessage type for key %s.", key)
				continue // 跳过这个项，继续下一个
			}

			mylog.Printf("richMediaMessage: %+v\n", richMediaMessage)
			_, err = apiv2.PostGroupMessage(context.TODO(), message.Params.GroupID.(string), richMediaMessage)
			if err != nil {
				mylog.Printf("发送 %s 信息失败_send_msg: %v", key, err)
				// 如果是真实错误，尝试发送错误提示
				if isRealFailure(err) {
					errorMsg := &dto.MessageToCreate{
						Content: fmt.Sprintf("图片发送失败: %v", err),
						MsgID:   messageID,
						MsgType: 0,
					}
					apiv2.PostGroupMessage(context.TODO(), message.Params.GroupID.(string), errorMsg)
				}
			}
			//发送成功回执
			SendResponse(client, err, &message)
		}
	case "guild":
		//用GroupID给ChannelID赋值,因为我们是把频道虚拟成了群
		message.Params.ChannelID = message.Params.GroupID.(string)
		// 使用RetrieveRowByIDv2还原真实的ChannelID
		RChannelID, err := idmap.RetrieveRowByIDv2(message.Params.ChannelID)
		if err != nil {
			mylog.Printf("error retrieving real UserID: %v", err)
		}
		message.Params.ChannelID = RChannelID
		handleSendGuildChannelMsg(client, api, apiv2, message)
	case "guild_private":
		//send_msg比具体的send_xxx少一层,其包含的字段类型在虚拟化场景已经失去作用
		//根据userid绑定得到的具体真实事件类型,这里也有多种可能性
		//1,私聊(但虚拟成了群),这里用群号取得需要的id
		//2,频道私聊(但虚拟成了私聊)这里传递2个nil,用user_id去推测channel_id和guild_id

		var channelIDPtr *string
		var GuildidPtr *string

		// 先尝试将GroupID断言为字符串
		if channelID, ok := message.Params.GroupID.(string); ok && channelID != "" {
			channelIDPtr = &channelID
			// 读取bolt数据库 通过ChannelID取回之前储存的guild_id
			if value, err := idmap.ReadConfigv2(*channelIDPtr, "guild_id"); err == nil && value != "" {
				GuildidPtr = &value
			} else {
				mylog.Printf("Error reading config: %v", err)
			}
		}

		if channelIDPtr == nil || GuildidPtr == nil {
			mylog.Printf("Value or ChannelID is empty or in error. Value: %v, ChannelID: %v", GuildidPtr, channelIDPtr)
		}

		handleSendGuildChannelPrivateMsg(client, api, apiv2, message, GuildidPtr, channelIDPtr)

	case "group_private":
		//私聊信息
		// 优先从 request_id/echo 中解析UserID（如果对方返回了request_id）
		var UserID int64
		var err error
		if echoStr, ok := resolveEchoToString(echoVal); ok {
			// 通过 echo->messageID->userID 反向映射查找真实的UserID
			if msgID := echo.GetMsgIDByKey(echoStr); msgID != "" {
				if uid := echo.GetUserIDByMsgID(msgID); uid > 0 {
					UserID = uid
				}
			}
		}
		// 如果仍未找到，则回退到 Params.UserID（兼容旧行为）
		if UserID == 0 {
			realUserIDStr, err := idmap.RetrieveRowByIDv2(message.Params.UserID.(string))
			if err != nil {
				mylog.Printf("Error reading config: %v", err)
				return
			}
			if uid, err := strconv.ParseInt(realUserIDStr, 10, 64); err == nil {
				UserID = uid
			} else {
				mylog.Printf("无法解析 realUserIDStr: %v", err)
				return
			}
		}

		// 解析消息内容
		messageText, foundItems := parseMessageContent(message.Params)

		// 使用 echo 获取消息ID
		var messageID string
		if echoStr, ok := echoVal.(string); ok {
			messageID = echo.GetMsgIDByKey(echoStr)
			mylog.Println("echo取私聊发信息对应的message_id:", messageID)
		}
		// 如果messageID为空，通过函数获取
		if messageID == "" {
			messageID = GetMessageIDByUseridOrGroupid(config.GetAppIDStr(), UserID)
			mylog.Println("通过GetMessageIDByUserid函数获取的message_id:", messageID)
		}
		mylog.Println("私聊发信息messageText:", messageText)
		//mylog.Println("foundItems:", foundItems)

		// 优先发送文本信息
		if messageText != "" {
			groupReply := generateGroupMessage(messageID, nil, messageText)

			// 进行类型断言
			groupMessage, ok := groupReply.(*dto.MessageToCreate)
			if !ok {
				mylog.Println("Error: Expected MessageToCreate type.")
				return
			}

			groupMessage.Timestamp = time.Now().Unix() // 设置时间戳
			_, err = apiv2.PostC2CMessage(context.TODO(), fmt.Sprint(UserID), groupMessage)
			if err != nil {
				mylog.Printf("发送文本私聊信息失败: %v", err)
				// 如果是真实错误，尝试发送错误提示
				if isRealFailure(err) {
					errorMsg := &dto.MessageToCreate{
						Content: fmt.Sprintf("消息发送失败: %v", err),
						MsgID:   messageID,
						MsgType: 0,
					}
					apiv2.PostC2CMessage(context.TODO(), fmt.Sprint(UserID), errorMsg)
				}
			}
			//发送成功回执
			SendResponse(client, err, &message)
		}

		// 遍历 foundItems 并发送每种信息
		for key, urls := range foundItems {
			var singleItem = make(map[string][]string)
			singleItem[key] = urls

			groupReply := generateGroupMessage(messageID, singleItem, "")

			// 进行类型断言
			richMediaMessage, ok := groupReply.(*dto.RichMediaMessage)
			if !ok {
				mylog.Printf("Error: Expected RichMediaMessage type for key %s.", key)
				continue
			}
			_, err = apiv2.PostC2CMessage(context.TODO(), fmt.Sprint(UserID), richMediaMessage)
			if err != nil {
				mylog.Printf("发送 %s 私聊信息失败: %v", key, err)
				// 如果是真实错误，尝试发送错误提示
				if isRealFailure(err) {
					errorMsg := &dto.MessageToCreate{
						Content: fmt.Sprintf("图片发送失败: %v", err),
						MsgID:   messageID,
						MsgType: 0,
					}
					apiv2.PostC2CMessage(context.TODO(), fmt.Sprint(UserID), errorMsg)
				}
			}
			//发送成功回执
			SendResponse(client, err, &message)
		}
	default:
		mylog.Printf("1Unknown message type: %s", msgType)
	}
}

// 通过user_id获取messageID
func GetMessageIDByUseridOrGroupid(appID string, userID interface{}) string {
	// 从appID和userID生成key
	var userid64 int64
	switch u := userID.(type) {
	case int:
		userid64 = int64(u)
	case int64:
		userid64 = u
	case float64:
		userid64 = int64(u)
	case string:
		var err error
		userid64, err = strconv.ParseInt(u, 10, 64)
		if err != nil {
			mylog.Printf("GetMessageIDByUseridOrGroupid: 无法解析字符串为int64: %s", u)
			return ""
		}
	default:
		// 可能需要处理其他类型或报错
		mylog.Printf("GetMessageIDByUseridOrGroupid: 未知类型 %T", userID)
		return ""
	}
	// 直接使用原始UserID构造key，不经过idmap转换
	// 因为存储时使用的就是原始UserID（见ProcessGroupMessage.go L101）
	key := appID + "_" + fmt.Sprint(userid64)
	msgID := echo.GetMsgIDByKey(key)
	mylog.Printf("查询: key[%s] -> msgID[%s]", key, msgID)
	return msgID
}
