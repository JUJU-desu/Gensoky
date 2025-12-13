package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hoshinonyaruko/gensokyo/mylog"
	"github.com/hoshinonyaruko/gensokyo/sys"
	"github.com/hoshinonyaruko/gensokyo/template"
	"gopkg.in/yaml.v3"
)

var (
	instance *Config
	mu       sync.Mutex
)

type Config struct {
	Version  int      `yaml:"version"`
	Settings Settings `yaml:"settings"`
}

type Settings struct {
	WsAddress              []string `yaml:"ws_address"`
	AppID                  uint64   `yaml:"app_id"`
	Token                  string   `yaml:"token"`
	ClientSecret           string   `yaml:"client_secret"`
	TextIntent             []string `yaml:"text_intent"`
	GlobalChannelToGroup   bool     `yaml:"global_channel_to_group"`
	GlobalPrivateToChannel bool     `yaml:"global_private_to_channel"`
	Array                  bool     `yaml:"array"`
	Server_dir             string   `yaml:"server_dir"`
	Lotus                  bool     `yaml:"lotus"`
	Port                   string   `yaml:"port"`
	WsToken                []string `yaml:"ws_token,omitempty"`         // 连接wss时使用,不是wss可留空 一一对应
	MasterID               []string `yaml:"master_id,omitempty"`        // 如果需要在群权限判断是管理员是,将user_id填入这里,master_id是一个文本数组
	EnableWsServer         bool     `yaml:"enable_ws_server,omitempty"` //正向ws开关
	WsServerToken          string   `yaml:"ws_server_token,omitempty"`  //正向ws token
	IdentifyFile           bool     `yaml:"identify_file"`              // 域名校验文件
	Crt                    string   `yaml:"crt"`
	Key                    string   `yaml:"key"`
	DeveloperLog           bool     `yaml:"developer_log"`
	LogLevel               string   `yaml:"log_level"` // 日志级别: error, warn, info, debug
	ImageLimit             int      `yaml:"image_sizelimit"`
	RemovePrefix           bool     `yaml:"remove_prefix"`
	BackupPort             string   `yaml:"backup_port"`
	DevlopAcDir            string   `yaml:"develop_access_token_dir"`
	RemoveAt               bool     `yaml:"remove_at"`
	DevBotid               string   `yaml:"develop_bot_id"`
	SandBoxMode            bool     `yaml:"sandbox_mode"`
	Title                  string   `yaml:"title"`
	HashID                 bool     `yaml:"hash_id"`
	TwoWayEcho             bool     `yaml:"twoway_echo"`
	UseRequestID           bool     `yaml:"use_requestid"`
	AutoReply              bool     `yaml:"auto_reply"`                  // 是否启用自动回复
	AutoReplyMessage       string   `yaml:"auto_reply_message"`          // 自动回复的消息内容
	CommandWhitelist       []string `yaml:"command_whitelist,omitempty"` // 指令白名单，只有这些指令会上报到ws服务器
	ConfigAutoReload       bool     `yaml:"config_auto_reload"`          // 配置文件热加载，检测到config.yml变动时自动重启
}

// LoadConfig 从文件中加载配置并初始化单例配置
func LoadConfig(path string) (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	// 如果单例已经被初始化了，直接返回
	if instance != nil {
		return instance, nil
	}

	configData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	conf := &Config{}
	err = yaml.Unmarshal(configData, conf)
	if err != nil {
		return nil, err
	}

	// 自动迁移旧配置 twoway_echo -> use_requestid（如果用户使用旧配置名）
	if err := migrateTwowayToUseRequestID(path, conf); err != nil {
		// 迁移失败不影响启动，但打印日志
		mylog.Printf("config migration warning: %v", err)
	}

	// 确保配置完整性
	if err := ensureConfigComplete(conf, path); err != nil {
		return nil, err
	}

	// 设置单例实例
	instance = conf
	return instance, nil
}

// 确保配置完整性
func ensureConfigComplete(conf *Config, path string) error {
	// 读取配置文件到缓冲区
	configData, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// 将现有的配置解析到结构体中
	currentConfig := &Config{}
	err = yaml.Unmarshal(configData, currentConfig)
	if err != nil {
		return err
	}

	// 解析默认配置模板到结构体中
	defaultConfig := &Config{}
	err = yaml.Unmarshal([]byte(template.ConfigTemplate), defaultConfig)
	if err != nil {
		return err
	}

	// 使用反射找出结构体中缺失的设置
	missingSettingsByReflection, err := getMissingSettingsByReflection(currentConfig, defaultConfig)
	if err != nil {
		return err
	}

	// 使用文本比对找出缺失的设置
	missingSettingsByText, err := getMissingSettingsByText(template.ConfigTemplate, string(configData))
	if err != nil {
		return err
	}

	// 合并缺失的设置
	allMissingSettings := mergeMissingSettings(missingSettingsByReflection, missingSettingsByText)

	// 如果存在缺失的设置，处理缺失的配置行
	if len(allMissingSettings) > 0 {
		fmt.Println("缺失的设置:", allMissingSettings)
		missingConfigLines, err := extractMissingConfigLines(allMissingSettings, template.ConfigTemplate)
		if err != nil {
			return err
		}

		// 将缺失的配置追加到现有配置文件
		err = appendToConfigFile(path, missingConfigLines)
		if err != nil {
			return err
		}

		fmt.Println("检测到配置文件缺少项。已经更新配置文件，正在重启程序以应用新的配置。")
		sys.RestartApplication()
	}

	return nil
}

// migrateTwowayToUseRequestID 将旧配置项 twoway_echo 升级为 use_requestid
// 如果配置文件中存在 twoway_echo 而不存在 use_requestid，会把 key 名替换为 use_requestid 并保持原值
func migrateTwowayToUseRequestID(path string, conf *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(data)
	reUse := regexp.MustCompile(`(?m)^\s*use_requestid\s*:`)
	reTwo := regexp.MustCompile(`(?m)^\s*twoway_echo\s*:`)
	if reTwo.MatchString(s) && !reUse.MatchString(s) {
		newContent := reTwo.ReplaceAllString(s, "use_requestid:")
		// 保持缩进：更精确替换保留前导空白
		reTwo2 := regexp.MustCompile(`(?m)^(\s*)twoway_echo(\s*:)`)
		newContent = reTwo2.ReplaceAllString(s, "${1}use_requestid${2}")
		if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
			return err
		}
		// 把内存里的值更新为 twoway_echo 的值（以防尚未派生到 UseRequestID）
		// 如果 TwoWayEcho 为 true，则设置 UseRequestID 为 true
		if conf != nil {
			conf.Settings.UseRequestID = conf.Settings.TwoWayEcho
		}
	}
	return nil
}

// mergeMissingSettings 合并由反射和文本比对找到的缺失设置
func mergeMissingSettings(reflectionSettings, textSettings map[string]string) map[string]string {
	for k, v := range textSettings {
		reflectionSettings[k] = v
	}
	return reflectionSettings
}

// getMissingSettingsByReflection 使用反射来对比结构体并找出缺失的设置
func getMissingSettingsByReflection(currentConfig, defaultConfig *Config) (map[string]string, error) {
	missingSettings := make(map[string]string)
	currentVal := reflect.ValueOf(currentConfig).Elem()
	defaultVal := reflect.ValueOf(defaultConfig).Elem()

	for i := 0; i < currentVal.NumField(); i++ {
		field := currentVal.Type().Field(i)
		yamlTag := field.Tag.Get("yaml")
		if yamlTag == "" || field.Type.Kind() == reflect.Int || field.Type.Kind() == reflect.Bool {
			continue // 跳过没有yaml标签的字段，或者字段类型为int或bool
		}
		yamlKeyName := strings.SplitN(yamlTag, ",", 2)[0]
		if isZeroOfUnderlyingType(currentVal.Field(i).Interface()) && !isZeroOfUnderlyingType(defaultVal.Field(i).Interface()) {
			missingSettings[yamlKeyName] = "missing"
		}
	}

	return missingSettings, nil
}

// getMissingSettingsByText compares settings in two strings line by line, looking for missing keys.
func getMissingSettingsByText(templateContent, currentConfigContent string) (map[string]string, error) {
	templateKeys := extractKeysFromString(templateContent)
	currentKeys := extractKeysFromString(currentConfigContent)

	missingSettings := make(map[string]string)
	for key := range templateKeys {
		if _, found := currentKeys[key]; !found {
			missingSettings[key] = "missing"
		}
	}

	return missingSettings, nil
}

// extractKeysFromString reads a string and extracts the keys (text before the colon).
func extractKeysFromString(content string) map[string]bool {
	keys := make(map[string]bool)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, ":") {
			key := strings.TrimSpace(strings.Split(line, ":")[0])
			keys[key] = true
		}
	}
	return keys
}

func extractMissingConfigLines(missingSettings map[string]string, configTemplate string) ([]string, error) {
	var missingConfigLines []string

	lines := strings.Split(configTemplate, "\n")
	for yamlKey := range missingSettings {
		found := false
		// Create a regex to match the line with optional spaces around the colon
		regexPattern := fmt.Sprintf(`^\s*%s\s*:\s*`, regexp.QuoteMeta(yamlKey))
		regex, err := regexp.Compile(regexPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %s", err)
		}

		for _, line := range lines {
			if regex.MatchString(line) {
				missingConfigLines = append(missingConfigLines, line)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("missing configuration for key: %s", yamlKey)
		}
	}

	return missingConfigLines, nil
}

func appendToConfigFile(path string, lines []string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("打开文件错误:", err)
		return err
	}
	defer file.Close()

	// 写入缺失的配置项
	for _, line := range lines {
		if _, err := file.WriteString("\n" + line); err != nil {
			fmt.Println("写入配置错误:", err)
			return err
		}
	}

	// 输出写入状态
	fmt.Println("配置已更新，写入到文件:", path)

	return nil
}

func isZeroOfUnderlyingType(x interface{}) bool {
	return reflect.DeepEqual(x, reflect.Zero(reflect.TypeOf(x)).Interface())
}

// UpdateConfig 将配置写入文件
func UpdateConfig(conf *Config, path string) error {
	data, err := yaml.Marshal(conf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// WriteYAMLToFile 将YAML格式的字符串写入到指定的文件路径
func WriteYAMLToFile(yamlContent string) error {
	// 获取当前执行的可执行文件的路径
	exePath, err := os.Executable()
	if err != nil {
		log.Println("Error getting executable path:", err)
		return err
	}

	// 获取可执行文件所在的目录
	exeDir := filepath.Dir(exePath)

	// 构建config.yml的完整路径
	configPath := filepath.Join(exeDir, "config.yml")

	// 写入文件
	os.WriteFile(configPath, []byte(yamlContent), 0644)

	sys.RestartApplication()
	return nil
}

// DeleteConfig 删除配置文件并创建一个新的配置文件模板
func DeleteConfig() error {
	// 获取当前执行的可执行文件的路径
	exePath, err := os.Executable()
	if err != nil {
		mylog.Println("Error getting executable path:", err)
		return err
	}

	// 获取可执行文件所在的目录
	exeDir := filepath.Dir(exePath)

	// 构建config.yml的完整路径
	configPath := filepath.Join(exeDir, "config.yml")

	// 删除配置文件
	if err := os.Remove(configPath); err != nil {
		mylog.Println("Error removing config file:", err)
		return err
	}

	// 获取内网IP地址
	ip, err := sys.GetLocalIP()
	if err != nil {
		mylog.Println("Error retrieving the local IP address:", err)
		return err
	}

	// 将 <YOUR_SERVER_DIR> 替换成实际的内网IP地址
	configData := strings.Replace(template.ConfigTemplate, "<YOUR_SERVER_DIR>", ip, -1)

	// 创建一个新的配置文件模板 写到配置
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		mylog.Println("Error writing config.yml:", err)
		return err
	}

	sys.RestartApplication()

	return nil
}

// 获取ws地址数组
func GetWsAddress() []string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.WsAddress
	}
	return nil // 返回nil，如果instance为nil
}

// 获取gensokyo服务的地址
func GetServer_dir() string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get upload directory.")
		return ""
	}
	return instance.Settings.Server_dir
}

// 获取DevBotid
func GetDevBotid() string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get DevBotid.")
		return "1234"
	}
	return instance.Settings.DevBotid
}

// 获取Develop_Acdir服务的地址
func GetDevelop_Acdir() string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get DevlopAcDir.")
		return ""
	}
	return instance.Settings.DevlopAcDir
}

// 获取lotus的值
func GetLotusValue() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get lotus value.")
		return false
	}
	return instance.Settings.Lotus
}

// 获取双向ehco
func GetTwoWayEcho() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get lotus value.")
		return false
	}
	return instance.Settings.TwoWayEcho
}

// GetUseRequestID returns whether to use requestID for echostr
// Backward compatible: if UseRequestID is false, fall back to TwoWayEcho
func GetUseRequestID() bool {
	mu.Lock()
	defer mu.Unlock()
	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get UseRequestID value.")
		return false
	}
	if instance.Settings.UseRequestID {
		return true
	}
	return instance.Settings.TwoWayEcho
}

// 获取HashID
func GetHashIDValue() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get hashid value.")
		return false
	}
	return instance.Settings.HashID
}

// 获取RemoveAt的值
func GetRemoveAt() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get RemoveAt value.")
		return false
	}
	return instance.Settings.RemoveAt
}

// 获取port的值
func GetPortValue() string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get port value.")
		return ""
	}
	return instance.Settings.Port
}

// 获取Array的值
func GetArrayValue() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get array value.")
		return false
	}
	return instance.Settings.Array
}

// 获取AppID
func GetAppID() uint64 {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.AppID
	}
	return 0 // or whatever default value you'd like to return if instance is nil
}

// 获取AppID String
func GetAppIDStr() string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return fmt.Sprintf("%d", instance.Settings.AppID)
	}
	return "0"
}

// 获取WsToken
func GetWsToken() []string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.WsToken
	}
	return nil // 返回nil，如果instance为nil
}

// 获取MasterID数组
func GetMasterID() []string {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return instance.Settings.MasterID
	}
	return nil // 返回nil，如果instance为nil
}

// 获取port的值
func GetEnableWsServer() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get port value.")
		return false
	}
	return instance.Settings.EnableWsServer
}

// 获取WsServerToken的值
func GetWsServerToken() string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get WsServerToken value.")
		return ""
	}
	return instance.Settings.WsServerToken
}

// 获取identify_file的值
func GetIdentifyFile() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get identify file name.")
		return false
	}
	return instance.Settings.IdentifyFile
}

// 获取crt路径
func GetCrtPath() string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get crt path.")
		return ""
	}
	return instance.Settings.Crt
}

// 获取key路径
func GetKeyPath() string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get key path.")
		return ""
	}
	return instance.Settings.Key
}

// 开发者日志
func GetDeveloperLog() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get developer log status.")
		return false
	}
	return instance.Settings.DeveloperLog
}

// GetImageLimit 返回 ImageLimit 的值
func GetImageLimit() int {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get image limit value.")
		return 0 // 或者返回一个默认的 ImageLimit 值
	}

	return instance.Settings.ImageLimit
}

// GetRemovePrefixValue 函数用于获取 remove_prefix 的配置值
func GetRemovePrefixValue() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get remove_prefix value.")
		return false // 或者可能是默认值，取决于您的应用程序逻辑
	}
	return instance.Settings.RemovePrefix
}

// GetLotusPort retrieves the LotusPort setting from your singleton instance.
func GetBackupPort() string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get LotusPort.")
		return ""
	}

	return instance.Settings.BackupPort
}

// GetAutoReply 获取是否启用自动回复
func GetAutoReply() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get AutoReply.")
		return false
	}

	return instance.Settings.AutoReply
}

// GetAutoReplyMessage 获取自动回复的消息内容
func GetAutoReplyMessage() string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get AutoReplyMessage.")
		return ""
	}

	return instance.Settings.AutoReplyMessage
}

// GetCommandWhitelist 获取指令白名单
func GetCommandWhitelist() []string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get CommandWhitelist.")
		return []string{}
	}

	return instance.Settings.CommandWhitelist
}

// IsCommandInWhitelist 检查消息是否在白名单内
// 如果白名单为空，则所有消息都通过
// 否则只有以白名单中的指令开头的消息才通过
func IsCommandInWhitelist(message string) bool {
	whitelist := GetCommandWhitelist()

	// 如果白名单为空，所有消息都通过
	if len(whitelist) == 0 {
		return true
	}

	// 去除消息前后空格
	message = strings.TrimSpace(message)

	// 检查消息是否以白名单中的任何指令开头
	for _, cmd := range whitelist {
		if strings.HasPrefix(message, cmd) {
			return true
		}
	}

	return false
}

// GetConfigAutoReload 获取是否启用配置文件热加载
func GetConfigAutoReload() bool {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		mylog.Println("Warning: instance is nil when trying to get ConfigAutoReload.")
		return false
	}

	return instance.Settings.ConfigAutoReload
}

// GetLogLevel 获取日志级别配置
func GetLogLevel() string {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		return "info" // 默认返回info级别
	}

	if instance.Settings.LogLevel == "" {
		return "info" // 如果配置为空，返回info
	}

	return instance.Settings.LogLevel
}

// WatchConfigFile 监听配置文件变化，检测到变化时自动重启程序
func WatchConfigFile(configPath string) {
	if !GetConfigAutoReload() {
		mylog.Println("配置文件热加载已禁用")
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		mylog.Printf("创建文件监听器失败: %v", err)
		return
	}

	// 获取配置文件的绝对路径
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		mylog.Printf("获取配置文件绝对路径失败: %v", err)
		return
	}

	go func() {
		defer watcher.Close()

		// 添加配置文件到监听列表
		err = watcher.Add(absPath)
		if err != nil {
			mylog.Printf("添加配置文件到监听失败: %v", err)
			return
		}

		mylog.Printf("配置文件热加载已启动，正在监听: %s", absPath)

		// 防抖动：避免短时间内多次触发重启
		var lastModTime time.Time
		debounceDelay := 2 * time.Second

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// 只处理写入和创建事件
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					now := time.Now()
					// 防抖动：如果距离上次修改时间小于延迟时间，则忽略
					if now.Sub(lastModTime) < debounceDelay {
						continue
					}
					lastModTime = now

					mylog.Printf("检测到配置文件变动: %s", event.Name)
					mylog.Println("等待2秒后重启程序以应用新配置...")

					// 延迟一下确保文件写入完成
					time.Sleep(debounceDelay)

					mylog.Println("正在重启程序...")
					sys.RestartApplication()
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				mylog.Printf("配置文件监听错误: %v", err)
			}
		}
	}()
}
