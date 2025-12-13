package echo

import (
	"strconv"
	"sync"
	"time"
)

type msgIDWithTime struct {
	msgID     string
	timestamp int64
}

type userIDWithTime struct {
	userID    int64
	timestamp int64
}

type pendingMessage struct {
	userID    int64
	msgID     string
	timestamp int64
}

type EchoMapping struct {
	mu                 sync.Mutex
	msgTypeMapping     map[string]string
	msgIDMapping       map[string]msgIDWithTime   // 带时间戳
	msgIDToUserIDMap   map[string]userIDWithTime  // 反向映射带时间戳
	groupLatestUserMap map[int64]userIDWithTime   // GroupID -> 最近的UserID（解决OneBot不传user_id的问题）
	groupPendingQueue  map[int64][]pendingMessage // GroupID -> 待处理消息队列（解决并发问题）
	lastCleanup        int64                      // 上次清理时间
}

var globalEchoMapping = &EchoMapping{
	msgTypeMapping:     make(map[string]string),
	msgIDMapping:       make(map[string]msgIDWithTime),
	msgIDToUserIDMap:   make(map[string]userIDWithTime),
	groupLatestUserMap: make(map[int64]userIDWithTime),
	groupPendingQueue:  make(map[int64][]pendingMessage),
	lastCleanup:        time.Now().Unix(),
}

func (e *EchoMapping) GenerateKey(appid string, s int64) string {
	return appid + "_" + strconv.FormatInt(s, 10)
}

// 添加echo对应的类型
func AddMsgType(appid string, s int64, msgType string) {
	key := globalEchoMapping.GenerateKey(appid, s)
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()
	globalEchoMapping.msgTypeMapping[key] = msgType
}

// 添加echo对应的messageid（带时间戳，自动清理过期数据）
func AddMsgID(appid string, s int64, msgID string) {
	key := globalEchoMapping.GenerateKey(appid, s)
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()

	now := time.Now().Unix()
	globalEchoMapping.msgIDMapping[key] = msgIDWithTime{
		msgID:     msgID,
		timestamp: now,
	}

	// 每10分钟清理一次过期数据（被动回复5分钟有效期，保留双倍时间）
	if now-globalEchoMapping.lastCleanup > 600 {
		go cleanupExpiredMappings()
		globalEchoMapping.lastCleanup = now
	}
}

// 根据给定的key获取消息类型
func GetMsgTypeByKey(key string) string {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()
	return globalEchoMapping.msgTypeMapping[key]
}

// 根据给定的key获取消息ID
func GetMsgIDByKey(key string) string {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()
	if data, ok := globalEchoMapping.msgIDMapping[key]; ok {
		return data.msgID
	}
	return ""
}

// 添加MessageID到UserID的映射（用于反向查询，带时间戳）
func AddMsgIDToUserID(msgID string, userID int64) {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()
	globalEchoMapping.msgIDToUserIDMap[msgID] = userIDWithTime{
		userID:    userID,
		timestamp: time.Now().Unix(),
	}
}

// 根据MessageID获取UserID
func GetUserIDByMsgID(msgID string) int64 {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()
	if data, ok := globalEchoMapping.msgIDToUserIDMap[msgID]; ok {
		return data.userID
	}
	return 0
}

// 记录GroupID的最新UserID（用于OneBot不传user_id时的降级方案）
func SetGroupLatestUser(groupID int64, userID int64) {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()
	globalEchoMapping.groupLatestUserMap[groupID] = userIDWithTime{
		userID:    userID,
		timestamp: time.Now().Unix(),
	}
}

// 添加待处理消息到群组队列（解决并发问题）
func AddGroupPendingMessage(groupID int64, userID int64, msgID string) {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()

	msg := pendingMessage{
		userID:    userID,
		msgID:     msgID,
		timestamp: time.Now().Unix(),
	}

	globalEchoMapping.groupPendingQueue[groupID] = append(globalEchoMapping.groupPendingQueue[groupID], msg)
}

// 获取并移除群组最早的待处理消息（FIFO，解决并发问题）
func PopGroupPendingMessage(groupID int64) (userID int64, msgID string) {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()

	queue, exists := globalEchoMapping.groupPendingQueue[groupID]
	if !exists || len(queue) == 0 {
		return 0, ""
	}

	// 获取队列第一个元素（最早的消息）
	msg := queue[0]

	// 移除第一个元素
	globalEchoMapping.groupPendingQueue[groupID] = queue[1:]

	// 如果队列为空，删除该entry
	if len(globalEchoMapping.groupPendingQueue[groupID]) == 0 {
		delete(globalEchoMapping.groupPendingQueue, groupID)
	}

	return msg.userID, msg.msgID
}

// 获取GroupID的最新UserID
func GetGroupLatestUser(groupID int64) int64 {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()
	if data, ok := globalEchoMapping.groupLatestUserMap[groupID]; ok {
		return data.userID
	}
	return 0
}

// 清理过期的映射数据（超过10分钟的数据）
func cleanupExpiredMappings() {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()

	now := time.Now().Unix()
	expireTime := int64(600) // 10分钟过期

	// 清理msgIDMapping中的过期数据
	for key, data := range globalEchoMapping.msgIDMapping {
		if now-data.timestamp > expireTime {
			delete(globalEchoMapping.msgIDMapping, key)
		}
	}

	// 清理msgIDToUserIDMap中的过期数据
	for msgID, data := range globalEchoMapping.msgIDToUserIDMap {
		if now-data.timestamp > expireTime {
			delete(globalEchoMapping.msgIDToUserIDMap, msgID)
		}
	}

	// 清理groupLatestUserMap中的过期数据
	for groupID, data := range globalEchoMapping.groupLatestUserMap {
		if now-data.timestamp > expireTime {
			delete(globalEchoMapping.groupLatestUserMap, groupID)
		}
	}

	// 清理groupPendingQueue中的过期数据
	for groupID, queue := range globalEchoMapping.groupPendingQueue {
		// 预分配容量避免重新分配
		newQueue := make([]pendingMessage, 0, len(queue))
		hasExpired := false
		for _, msg := range queue {
			if now-msg.timestamp <= expireTime {
				newQueue = append(newQueue, msg)
			} else {
				hasExpired = true
			}
		}
		// 只有在确实有过期数据时才更新或删除
		if hasExpired {
			if len(newQueue) > 0 {
				globalEchoMapping.groupPendingQueue[groupID] = newQueue
			} else {
				delete(globalEchoMapping.groupPendingQueue, groupID)
			}
		}
	}
}

// AddMsgIDWithKey 使用全局 key（非数值 s）来记录消息ID，用于 request_id 模式
func AddMsgIDWithKey(key string, msgID string) {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()
	now := time.Now().Unix()
	globalEchoMapping.msgIDMapping[key] = msgIDWithTime{
		msgID:     msgID,
		timestamp: now,
	}
	if now-globalEchoMapping.lastCleanup > 600 {
		go cleanupExpiredMappings()
		globalEchoMapping.lastCleanup = now
	}
}

// AddMsgTypeWithKey 使用全局 key（非数值 s）来记录消息类型，用于 request_id 模式
func AddMsgTypeWithKey(key string, msgType string) {
	globalEchoMapping.mu.Lock()
	defer globalEchoMapping.mu.Unlock()
	globalEchoMapping.msgTypeMapping[key] = msgType
}
