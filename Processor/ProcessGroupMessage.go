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

// ProcessGroupMessage 处理群组消息
func (p *Processors) ProcessGroupMessage(data *dto.WSGroupATMessageData) error {
	// 获取s（保留以防需要）

	// 转换at
	messageText := handlers.RevertTransformedText(data)

	// 转换appid
	AppIDString := strconv.FormatUint(p.Settings.AppID, 10)

	// 构造echostr（使用非自增 requestID 以避免重用 s）
	requestID := requestid.NewRequestID()
	echostr := AppIDString + "_" + requestID

	// 映射str的GroupID到int
	GroupID64, err := idmap.StoreIDv2(data.GroupID)
	if err != nil {
		return fmt.Errorf("failed to convert ChannelID to int: %v", err)
	}

	// 映射str的userid到int
	userid64, err := idmap.StoreIDv2(data.Author.ID)
	if err != nil {
		mylog.Printf("Error storing ID: %v", err)
		return nil
	}
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
		GroupID:     GroupID64,
		MessageType: "group",
		PostType:    "message",
		SelfID:      int64(p.Settings.AppID),
		UserID:      userid64,
		Sender: Sender{
			Nickname: "",
			UserID:   userid64,
		},
		SubType: "normal",
		Time:    time.Now().Unix(),
		Avatar:  "",
	}
	// 根据条件判断设置 Echo 或 RequestID
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
	// 将当前 requestID(非自增) 以及 appid 映射到 message（用于被动回复）
	echo.AddMsgIDWithKey(echostr, data.ID)
	echo.AddMsgTypeWithKey(echostr, "group")
	// 同时保存裸request_id到映射（兼容某些服务端只返回裸request_id）
	if idx := strings.Index(echostr, "_"); idx >= 0 {
		bare := echostr[idx+1:]
		echo.AddMsgIDWithKey(bare, data.ID)
		echo.AddMsgTypeWithKey(bare, "group")
	}
	//为不支持双向echo的ob服务端映射 - 使用UserID避免并发时相互覆盖
	echo.AddMsgID(AppIDString, userid64, data.ID)
	echo.AddMsgType(AppIDString, userid64, "group")
	mylog.Printf("存储映射: UserID[%d] -> MsgID[%s]", userid64, data.ID)
	//建立反向映射: MessageID -> UserID (原始ID和转换后的int ID都需要映射)
	echo.AddMsgIDToUserID(data.ID, userid64)                 // 原始MessageID
	echo.AddMsgIDToUserID(fmt.Sprint(messageID), userid64)   // 转换后的int MessageID
	echo.AddMsgIDToUserID(fmt.Sprint(messageID64), userid64) // int64格式
	mylog.Printf("存储反向映射: MsgID[%s/%d/%d] -> UserID[%d]", data.ID, messageID, messageID64, userid64)
	//记录群的最新UserID（关键！用于OneBot不传user_id时的降级方案）
	echo.SetGroupLatestUser(GroupID64, userid64)
	mylog.Printf("记录群[%d]的最新用户: UserID[%d]", GroupID64, userid64)
	// 只有当消息内容非空时，并且没有启用request_id时，才添加到待处理消息队列（避免空消息或非指令消息干扰队列）
	if messageText != "" && strings.TrimSpace(messageText) != "" {
		if !config.GetUseRequestID() {
			echo.AddGroupPendingMessage(GroupID64, userid64, data.ID)
			mylog.Printf("添加待处理消息到队列: GroupID[%d], UserID[%d], MsgID[%s]", GroupID64, userid64, data.ID)
		} else {
			mylog.Printf("已启用request_id，跳过加入待处理队列: GroupID[%d], UserID[%d]", GroupID64, userid64)
		}
	} else {
		mylog.Printf("跳过空消息，不加入队列: GroupID[%d], UserID[%d]", GroupID64, userid64)
	}
	//储存当前群或频道号的类型
	idmap.WriteConfigv2(fmt.Sprint(GroupID64), "type", "group")
	echo.AddMsgType(AppIDString, GroupID64, "group")

	// 检查消息是否在白名单内
	isInWhitelist := config.IsCommandInWhitelist(messageText)
	mylog.Printf("消息内容: [%s], 是否在白名单: %v", messageText, isInWhitelist)

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

	// 调试
	PrintStructWithFieldNames(groupMsg)

	// Convert OnebotGroupMessage to map and send
	groupMsgMap := structToMap(groupMsg)
	//上报信息到onebotv11应用端(正反ws)
	mylog.Println("消息在白名单内，上报到ws服务器")
	p.BroadcastMessageToAll(groupMsgMap)
	return nil
}
