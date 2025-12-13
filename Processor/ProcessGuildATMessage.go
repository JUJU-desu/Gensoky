// 处理收到的信息事件
package Processor

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hoshinonyaruko/gensokyo/config"
	"github.com/hoshinonyaruko/gensokyo/echo"
	"github.com/hoshinonyaruko/gensokyo/handlers"
	"github.com/hoshinonyaruko/gensokyo/idmap"
	"github.com/hoshinonyaruko/gensokyo/mylog"

	"github.com/hoshinonyaruko/gensokyo/requestid"
	"github.com/tencent-connect/botgo/dto"
)

// ProcessGuildATMessage 处理消息，执行逻辑并可能使用 api 发送响应
func (p *Processors) ProcessGuildATMessage(data *dto.WSATMessageData) error {
	if !p.Settings.GlobalChannelToGroup {
		// 将时间字符串转换为时间戳
		t, err := time.Parse(time.RFC3339, string(data.Timestamp))
		if err != nil {
			return fmt.Errorf("error parsing time: %v", err)
		}
		//获取s（保留以防需要）
		//转换at
		messageText := handlers.RevertTransformedText(data)

		// 检测单纯@bot的情况（内容为空或只有空格）
		if messageText == "" || strings.TrimSpace(messageText) == "" {
			mylog.Println("检测到单纯@bot，自动回复提示信息")
			// 构造友好的提示消息
			replyMsg := &dto.MessageToCreate{
				Content: "你好！我是机器人，请问有什么可以帮助你的吗？\n你可以使用 /help 查看可用命令。",
				MsgID:   data.ID,
			}
			// 发送回复到频道
			if _, err := p.Api.PostMessage(context.TODO(), data.ChannelID, replyMsg); err != nil {
				mylog.Printf("发送@bot提示消息失败: %v", err)
			} else {
				mylog.Println("已发送@bot提示消息")
			}
			// 提前返回，不继续处理
			return nil
		}

		//转换appid
		AppIDString := strconv.FormatUint(p.Settings.AppID, 10)
		// 构造echostr（使用非自增 request_id）
		requestID := requestid.NewRequestID()
		echostr := AppIDString + "_" + requestID
		//映射str的userid到int
		userid64, err := idmap.StoreIDv2(data.Author.ID)
		if err != nil {
			mylog.Printf("Error storing ID: %v", err)
			return nil
		}
		// 如果在Array模式下, 则处理Message为Segment格式
		var segmentedMessages interface{} = messageText
		if config.GetArrayValue() {
			segmentedMessages = handlers.ConvertToSegmentedMessage(data)
		}
		// 处理onebot_channel_message逻辑
		onebotMsg := OnebotChannelMessage{
			ChannelID:   data.ChannelID,
			GuildID:     data.GuildID,
			Message:     segmentedMessages,
			RawMessage:  messageText,
			MessageID:   data.ID,
			MessageType: "guild",
			PostType:    "message",
			SelfID:      int64(p.Settings.AppID),
			UserID:      userid64,
			SelfTinyID:  "",
			Sender: Sender{
				Nickname: data.Member.Nick,
				TinyID:   "",
				UserID:   userid64,
			},
			SubType: "channel",
			Time:    t.Unix(),
			Avatar:  data.Author.Avatar,
		}
		// 根据条件判断设置 Echo 或 request_id
		if config.GetUseRequestID() {
			onebotMsg.RequestID = echostr
		} else {
			onebotMsg.Echo = echostr
		}
		// 获取MasterID数组
		masterIDs := config.GetMasterID()

		// 判断userid64是否在masterIDs数组里
		isMaster := false
		for _, id := range masterIDs {
			if strconv.FormatInt(userid64, 10) == id {
				isMaster = true
				break
			}
		}

		// 根据isMaster的值为groupMsg的Sender赋值role字段
		if isMaster {
			onebotMsg.Sender.Role = "owner"
		} else {
			onebotMsg.Sender.Role = "member"
		}
		//将当前s和appid和message进行映射
		echo.AddMsgIDWithKey(echostr, data.ID)
		echo.AddMsgTypeWithKey(echostr, "guild")
		if idx := strings.Index(echostr, "_"); idx >= 0 {
			bare := echostr[idx+1:]
			echo.AddMsgIDWithKey(bare, data.ID)
			echo.AddMsgTypeWithKey(bare, "guild")
		}
		//为不支持双向echo的ob11服务端映射
		echo.AddMsgID(AppIDString, userid64, data.ID)
		echo.AddMsgType(AppIDString, userid64, "guild")
		//储存当前群或频道号的类型
		idmap.WriteConfigv2(data.ChannelID, "type", "guild")
		//todo 完善频道转换

		// 检查消息是否在白名单内
		isInWhitelist := config.IsCommandInWhitelist(messageText)
		mylog.Printf("频道消息内容: [%s], 是否在白名单: %v", messageText, isInWhitelist)

		// 如果不在白名单内，使用自动回复并跳过上报
		if !isInWhitelist && config.GetAutoReply() {
			autoReplyMsg := config.GetAutoReplyMessage()
			if autoReplyMsg != "" {
				mylog.Printf("消息不在白名单内，使用自动回复: %s", autoReplyMsg)
				replyMsg := &dto.MessageToCreate{
					Content: autoReplyMsg,
					MsgID:   data.ID,
				}
				if _, err := p.Api.PostMessage(context.TODO(), data.ChannelID, replyMsg); err != nil {
					mylog.Printf("发送自动回复失败: %v", err)
				} else {
					mylog.Println("自动回复发送成功，消息不上报到ws服务器")
				}
			}
			return nil // 不上报到ws服务器
		}

		//调试
		PrintStructWithFieldNames(onebotMsg)

		// 将 onebotMsg 结构体转换为 map[string]interface{}
		msgMap := structToMap(onebotMsg)

		//上报信息到onebotv11应用端(正反ws)
		mylog.Println("频道消息在白名单内，上报到ws服务器")
		p.BroadcastMessageToAll(msgMap)
	} else {
		// GlobalChannelToGroup为true时的处理逻辑
		//将频道转化为一个群
		// 获取s（保留但不用于 echostr，因为使用 request_id）
		//将channelid写入ini,可取出guild_id
		ChannelID64, err := idmap.StoreIDv2(data.ChannelID)
		if err != nil {
			mylog.Printf("Error storing ID: %v", err)
			return nil
		}
		//转成int再互转
		idmap.WriteConfigv2(fmt.Sprint(ChannelID64), "guild_id", data.GuildID)
		//转换at和图片
		messageText := handlers.RevertTransformedText(data)

		// 检测单纯@bot的情况（内容为空或只有空格）
		if messageText == "" || strings.TrimSpace(messageText) == "" {
			mylog.Println("检测到单纯@bot（GlobalChannelToGroup模式），自动回复提示信息")
			// 构造友好的提示消息
			replyMsg := &dto.MessageToCreate{
				Content: "你好！我是机器人，请问有什么可以帮助你的吗？\n你可以使用 /help 查看可用命令。",
				MsgID:   data.ID,
			}
			// 以群消息方式发送回复
			if _, err := p.Apiv2.PostGroupMessage(context.TODO(), data.GroupID, replyMsg); err != nil {
				mylog.Printf("发送@bot提示消息失败: %v", err)
			} else {
				mylog.Println("已发送@bot提示消息")
			}
			// 提前返回，不继续处理
			return nil
		}

		//转换appid
		AppIDString := strconv.FormatUint(p.Settings.AppID, 10)
		// 构造echostr（使用非自增 request_id）
		requestID := requestid.NewRequestID()
		echostr := AppIDString + "_" + requestID
		//映射str的userid到int
		userid64, err := idmap.StoreIDv2(data.Author.ID)
		if err != nil {
			mylog.Printf("Error storing ID: %v", err)
			return nil
		}
		//userid := int(userid64)
		//映射str的messageID到int
		messageID64, err := idmap.StoreIDv2(data.ID)
		if err != nil {
			mylog.Printf("Error storing ID: %v", err)
			return nil
		}
		messageID := int(messageID64)
		// 如果在Array模式下, 则处理Message为Segment格式
		var segmentedMessages interface{} = messageText
		if config.GetArrayValue() {
			segmentedMessages = handlers.ConvertToSegmentedMessage(data)
		}
		groupMsg := OnebotGroupMessage{
			RawMessage:  messageText,
			Message:     segmentedMessages,
			MessageID:   messageID,
			GroupID:     ChannelID64,
			MessageType: "group",
			PostType:    "message",
			SelfID:      int64(p.Settings.AppID),
			UserID:      userid64,
			Sender: Sender{
				Nickname: data.Member.Nick,
				UserID:   userid64,
			},
			SubType: "normal",
			Time:    time.Now().Unix(),
			Avatar:  data.Author.Avatar,
		}
		// 根据条件判断设置 Echo 或 request_id
		if config.GetUseRequestID() {
			groupMsg.RequestID = echostr
		} else {
			groupMsg.Echo = echostr
		}
		// 获取MasterID数组
		masterIDs := config.GetMasterID()

		// 判断userid64是否在masterIDs数组里
		isMaster := false
		for _, id := range masterIDs {
			if strconv.FormatInt(userid64, 10) == id {
				isMaster = true
				break
			}
		}

		// 根据isMaster的值为groupMsg的Sender赋值role字段
		if isMaster {
			groupMsg.Sender.Role = "owner"
		} else {
			groupMsg.Sender.Role = "member"
		}
		//将当前s和appid和message进行映射
		echo.AddMsgIDWithKey(echostr, data.ID)
		echo.AddMsgTypeWithKey(echostr, "guild")
		if idx := strings.Index(echostr, "_"); idx >= 0 {
			bare := echostr[idx+1:]
			echo.AddMsgIDWithKey(bare, data.ID)
			echo.AddMsgTypeWithKey(bare, "guild")
		}
		//为不支持双向echo的ob服务端映射 - 使用UserID避免并发时相互覆盖
		echo.AddMsgID(AppIDString, userid64, data.ID)
		echo.AddMsgType(AppIDString, userid64, "guild")
		//同时保留ChannelID映射作为备用(适用于单用户快速回复场景)
		echo.AddMsgID(AppIDString, ChannelID64, data.ID)
		echo.AddMsgType(AppIDString, ChannelID64, "guild")
		//储存当前群或频道号的类型
		idmap.WriteConfigv2(fmt.Sprint(ChannelID64), "type", "guild")

		// 检查消息是否在白名单内（GlobalChannelToGroup模式）
		isInWhitelist := config.IsCommandInWhitelist(messageText)
		mylog.Printf("频道消息(GlobalChannelToGroup): [%s], 是否在白名单: %v", messageText, isInWhitelist)

		// 如果不在白名单内，使用自动回复并跳过上报
		if !isInWhitelist && config.GetAutoReply() {
			autoReplyMsg := config.GetAutoReplyMessage()
			if autoReplyMsg != "" {
				mylog.Printf("消息不在白名单内，使用自动回复: %s", autoReplyMsg)
				replyMsg := &dto.MessageToCreate{
					Content: autoReplyMsg,
					MsgID:   data.ID,
				}
				if _, err := p.Apiv2.PostGroupMessage(context.TODO(), data.GroupID, replyMsg); err != nil {
					mylog.Printf("发送自动回复失败: %v", err)
				} else {
					mylog.Println("自动回复发送成功，消息不上报到ws服务器")
				}
			}
			return nil // 不上报到ws服务器
		}

		//调试
		PrintStructWithFieldNames(groupMsg)

		// Convert OnebotGroupMessage to map and send
		groupMsgMap := structToMap(groupMsg)
		//上报信息到onebotv11应用端(正反ws)
		mylog.Println("频道消息(GlobalChannelToGroup)在白名单内，上报到ws服务器")
		p.BroadcastMessageToAll(groupMsgMap)

	}

	return nil
}
