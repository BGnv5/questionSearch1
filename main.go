package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// 主响应结构体
type ExamResponse struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    Data   `json:"data"`
}

// 数据部分
type Data struct {
	Items     map[string]QuestionItem `json:"items"`
	Exam      ExamInfo                `json:"exam"`
}

// 题目项
type QuestionItem struct {
	UUID       string          `json:"uuid"`
	Type       string          `json:"type"`
	Title      string          `json:"title"`
	Score      interface{}     `json:"score"` // 使用interface{}处理字符串和数字
	Status     int             `json:"status"`
	Answer     []string        `json:"answer"`
	ShowAnswer string          `json:"show_answer"`
	Choices    []Choice        `json:"choices"`
	TestResult TestResult      `json:"test_result"`
	Difficulty string 		   `json:"difficulty"`
}

// 选项
type Choice struct {
	Operator  string `json:"operator"`
	Title     string `json:"title"`
	IsCorrect bool   `json:"isCorrect"`
	IsChecked bool   `json:"isChecked"`
}

// 测试结果 - 使用interface{}处理不同类型
type TestResult struct {
	Status        string      `json:"status"`
	Answer        interface{} `json:"answer"` // 可能是数组或单个值
	QuestionId    string      `json:"question_id"`
	Type          string      `json:"type"`
	Score         interface{} `json:"score"` // 可能是字符串或数字
	UpdateTime    string      `json:"update_time"`
	ShowUserAnswer string     `json:"show_user_answer"`
}

// 考试信息
type ExamInfo struct {
	ID          int    `json:"id"`
	UUID        string `json:"uuid"`
	Title       string `json:"title"`
	CategoryTitle string `json:"category_title"`
	QuestionCount int   `json:"question_count"`
	TotalScore  string `json:"total_score"`
	PassScore   string `json:"pass_score"`
	MaxDuration int    `json:"max_duration"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
}

type QuestionCategory struct {
	TypeName string
	Questions []QuestionItem
}

// 模板数据
type TemplateData struct {
	Results         []Result
	Keyword         string
	SelectedType    string
	ResultCount     int
	SearchPerformed bool
	Message         string
	MessageType     string // success, error, warning
}

// 获取题型中文名称
func getQuestionTypeName(qType string) string {
	switch qType {
	case "single_choice":
		return "单选题"
	case "choice":
		return "多选题"
	case "determine":
		return "判断题"
	default:
		return qType
	}
}

// 获取难度中文名称
func getDifficultyName(difficulty string) string {
	switch difficulty {
	case "simple":
		return "简单"
	case "normal":
		return "中等"
	case "difficulty":
		return "困难"
	case "quite_difficulty":
		return "较难"
	default:
		return difficulty
	}
}


// 辅助函数：安全获取分数值
func getScore(score interface{}) string {
	switch v := score.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	default:
		return "0"
	}
}

// 清理HTML标签和特殊字符
func cleanText(text string) string {
	text = strings.ReplaceAll(text, "<p>", "")
	text = strings.ReplaceAll(text, "</p>", "")
	text = strings.ReplaceAll(text, "&ldquo", "")
	text = strings.ReplaceAll(text, "&rdquo", "")
	text = strings.ReplaceAll(text, "&ge", "")
	text = strings.ReplaceAll(text, "&rsquo", "")
	text = strings.ReplaceAll(text, "&nbsp", "")
	text = strings.TrimSpace(text)
	return text
}

// 修改main函数为Vercel适配的Handler
func Handler(w http.ResponseWriter, r *http.Request) {

	// 设置路由处理
	switch r.URL.Path {
	case "/":
		handleIndex(w, r)
	case "/search":
		handleSearch(w, r)
	default:
		// 对于其他路径，重定向到首页
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func main() {
	// 设置路由
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/search", handleSearch)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("🚀 题目搜索系统启动成功!\n")
	fmt.Printf("📖 请打开浏览器访问: http://localhost:%s\n", port)
	fmt.Printf("⏹️  按 Ctrl+C 停止服务器\n")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("❌ 启动服务器失败: %v\n", err)
	}


}

type SearchConfig struct {
	Keyword string
	Type    string
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析错误: "+err.Error(), http.StatusBadRequest)
		return
	}

	config := SearchConfig{
		Keyword: strings.TrimSpace(r.FormValue("keyword")),
		Type:    r.FormValue("question_type"),
	}

	// 检查是否选择了题型
	if config.Type == "" {
		tmpl, err := template.ParseFiles("./root.html")
		if err != nil {
			http.Error(w, "模板解析错误: "+err.Error(), http.StatusInternalServerError)
			return
		}

		data := TemplateData{
			Keyword:      config.Keyword,
			SelectedType: "single_choice", // 默认选中单选题
			Message:      "⚠️ 请选择一种题型！",
			MessageType:  "warning",
		}

		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "模板渲染错误: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	questions, err := searchQuestions(config)
	if err != nil {
		http.Error(w, "搜索错误: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFiles("./root.html")
	if err != nil {
		http.Error(w, "模板解析错误: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := TemplateData{
		Results:         questions,
		Keyword:         config.Keyword,
		SelectedType:    config.Type,
		ResultCount:     len(questions),
		SearchPerformed: true,
	}

	if len(questions) == 0 {
		data.Message = "🔍 没有找到相关题目，请尝试其他关键词"
		data.MessageType = "warning"
	} else {
		// 显示最匹配的结果信息
		data.Message = fmt.Sprintf("✅ 搜索完成！找到 %d 条相关题目", len(questions))
		data.MessageType = "success"
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "模板渲染错误: "+err.Error(), http.StatusInternalServerError)
	}
}

type Result struct {
	Title 	  string
	Info   	  string
	Operator  []string
	Relevance float64
	Type      string
}

func searchQuestions(config SearchConfig) ([]Result, error) {
	var allQuestions []Result
	var category *QuestionCategory
	questionCategories := getQuestionCategories()

	switch config.Type {
	case "single_choice":
		if _, exists := questionCategories["单选题"]; exists {
			category = questionCategories["单选题"]
		}
	case "choice":
		if _, exists := questionCategories["多选题"]; exists {
			category = questionCategories["多选题"]
		}
	case "determine":
		if _, exists := questionCategories["判断题"]; exists {
			category = questionCategories["判断题"]
		}
	default:
		return nil, fmt.Errorf("无效的题型选择")
	}

	for _, question := range category.Questions {
		var res Result
		res.Title = fmt.Sprintf(" %s\n", cleanText(question.Title))
		res.Info =  fmt.Sprintf(" 难度: %s | 分值: %s | 正确答案: %s\n", getDifficultyName(question.Difficulty), getScore(question.Score), question.ShowAnswer)
		res.Type = getQuestionTypeName(config.Type)
		if len(question.Choices) > 0 {
			// 打印选项
			for _, choice := range question.Choices {
				correctMark := ""
				if choice.IsCorrect {
					correctMark = " ✓"
				}
				res.Operator = append(res.Operator, fmt.Sprintf("  %s. %s%s\n", choice.Operator, cleanText(choice.Title), correctMark))
			}
		}

		allQuestions = append(allQuestions, res)
	}

	// 计算相关性分数
	for i := range allQuestions {
		allQuestions[i].Relevance = calculateRelevance(allQuestions[i].Title, config.Keyword)
	}

	// 按相关性从高到低排序
	sort.Slice(allQuestions, func(i, j int) bool {
		if allQuestions[i].Relevance == allQuestions[j].Relevance {
			// 如果相关性相同，按内容长度排序（较短的可能更相关）
			return len(allQuestions[i].Title) < len(allQuestions[j].Title)
		}
		return allQuestions[i].Relevance > allQuestions[j].Relevance
	})

	// 如果有关键词，只返回相关性大于0的结果
	if config.Keyword != "" {
		var filtered []Result
		for _, q := range allQuestions {
			if q.Relevance > 0.5 { // 设置阈值，只返回相关性较高的结果
				filtered = append(filtered, q)
			}
		}

		// 如果过滤后结果太多，只返回前20个最相关的结果
		if len(filtered) > 20 {
			filtered = filtered[:20]
		}

		return filtered, nil
	}

	return allQuestions, nil
}

func calculateRelevance(question, keyword string) float64 {
	if keyword == "" {
		return 1.0 // 如果没有关键词，所有问题都显示
	}

	questionLower := strings.ToLower(question)
	keywordLower := strings.ToLower(keyword)

	// 1. 完全匹配最高分
	if questionLower == keywordLower {
		return 1.0
	}

	// 2. 开头匹配高分
	if strings.HasPrefix(questionLower, keywordLower) {
		return 0.95
	}

	// 3. 包含完整关键词
	if strings.Contains(questionLower, keywordLower) {
		// 检查是否在单词边界
		index := strings.Index(questionLower, keywordLower)
		if index > 0 {
			prevChar := rune(questionLower[index-1])
			if unicode.IsSpace(prevChar) || unicode.IsPunct(prevChar) {
				return 0.9
			}
		}
		return 0.8
	}

	// 4. 分词匹配
	keywordWords := strings.Fields(keywordLower)
	questionWords := strings.Fields(questionLower)

	if len(keywordWords) == 0 {
		return 0
	}

	// 计算匹配的单词数量和位置
	matchedWords := 0
	exactWordMatches := 0

	for _, kw := range keywordWords {
		found := false
		for _, qw := range questionWords {
			if qw == kw {
				exactWordMatches++
				found = true
				break
			} else if strings.Contains(qw, kw) {
				matchedWords++
				found = true
				break
			}
		}
		if !found {
			// 如果有任何一个关键词完全没匹配，相关性降低
			return 0.3
		}
	}

	// 计算基础分数
	baseScore := float64(matchedWords+exactWordMatches) / float64(len(keywordWords))

	// 精确单词匹配加分
	exactBonus := float64(exactWordMatches) * 0.1

	return baseScore*0.7 + exactBonus
}

// 处理首页
func handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("./root.html")
	if err != nil {
		http.Error(w, "模板解析错误: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := TemplateData{
		SelectedType: "single_choice",	// 默认选中单选题
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "模板渲染错误: "+err.Error(), http.StatusInternalServerError)
	}
}

func getQuestionCategories()  map[string]*QuestionCategory {
	// 读取文件
	_, allQuestions := readFile()

	// 将题目转换为切片以便排序
	questionsSlice := make([]QuestionItem, 0, len(allQuestions))
	for _, question := range allQuestions {
		questionsSlice = append(questionsSlice, question)
	}

	// 按题型分类题目
	questionCategories := make(map[string]*QuestionCategory)
	for _, question := range questionsSlice {
		typeName := getQuestionTypeName(question.Type)
		if _, exists := questionCategories[typeName]; !exists {
			questionCategories[typeName] = &QuestionCategory{
				TypeName:  typeName,
				Questions: make([]QuestionItem, 0),
			}
		}
		questionCategories[typeName].Questions = append(questionCategories[typeName].Questions, question)
	}

	return questionCategories
}

func readFile() (error, map[string]QuestionItem) {
	// 读取文件
	content, err := ioutil.ReadFile("./RawFile.txt")
	if err != nil {
		log.Fatal("读取文件失败:", err)
	}

	// 按行分割内容
	lines := strings.Split(string(content), "\n")

	// 用于存储所有题目
	allQuestions := make(map[string]QuestionItem)

	// 处理每一行JSON数据
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// 解析JSON
		var examResponse ExamResponse
		err = json.Unmarshal([]byte(line), &examResponse)
		if err != nil {
			log.Fatalf("解析JSON失败: %v\n\n", err)
			continue
		}

		newQuestion := 0
		for uuid, item := range examResponse.Data.Items {
			if _, exists := allQuestions[uuid]; !exists {
				allQuestions[uuid] = item
				newQuestion++
			}
		}
		fmt.Printf("\n=== 第%d条数据遍历完成, 总题目数: %d, 新增题数:%d ===\n", i+1, len(examResponse.Data.Items), newQuestion)
	}
	return err, allQuestions
}


