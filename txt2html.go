package main

import (
	"bufio"
	"bytes"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const targetHTMLSize = 1024 * 1024 // 目标HTML文件大小：1MB
const readBufferSize = 4096         // 读取缓冲区大小

// HTML模板数据结构
type TemplateData struct {
	Content      string
	FileName     string
	TotalChunks  int
	CurrentChunk int
}

// HTML模板内容 - 支持左右两侧展示背景颜色自定义
const htmlTemplate = ` + "`" + `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.FileName}} - 第{{.CurrentChunk}}部分</title>
    <style>
        :root {
            --left-bg: #f5f5f5;   /* 左侧默认背景 */
            --center-bg: #ffffff; /* 中央内容背景 */
            --right-bg: #f5f5f5;  /* 右侧默认背景 */
            --center-max-width: 1000px;
        }
        /* 使用线性渐变在页面两侧显示可配置颜色，中间使用中心背景色 */
        body {
            --g-left: calc(50% - var(--center-max-width) / 2);
            --g-right: calc(50% + var(--center-max-width) / 2);
            background: linear-gradient(to right,
                        var(--left-bg) 0px var(--g-left),
                        var(--center-bg) var(--g-left) var(--g-right),
                        var(--right-bg) var(--g-right) 100%);
            color: #333;
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            padding: 20px;
            margin: 0;
            font-size: 16px;
        }
        .controls {
            margin-bottom: 20px;
            padding: 15px;
            background-color: #f5f5f5;
            border-radius: 8px;
            display: flex;
            flex-wrap: wrap;
            gap: 15px;
            align-items: center;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .control-section {
            display: flex;
            flex-direction: column;
            gap: 8px;
        }
        .control-group {
            display: flex;
            gap: 10px;
            align-items: center;
        }
        button {
            background-color: #e0e0e0;
            color: #333;
            border: none;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
            transition: background-color 0.3s;
            font-size: 16px;
        }
        button:hover {
            background-color: #ccc;
        }
        .page-center {
            max-width: var(--center-max-width);
            margin: 0 auto;
            padding: 20px;
        }
        .content {
            white-space: pre-wrap;
            word-wrap: break-word;
            padding: 25px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            min-height: 300px;
            transition: background-color 0.3s, color 0.3s, line-height 0.3s;
            line-height: 1.6; /* 默认行距 */
            background-color: var(--center-bg);
        }
        .chunk-info {
            color: #666;
            font-size: 0.9em;
            margin-top: 10px;
            width: 100%;
            text-align: right;
        }
        .color-options {
            display: flex;
            gap: 8px;
            align-items: center;
        }
        .color-preview {
            width: 20px;
            height: 20px;
            border-radius: 4px;
            border: 1px solid rgba(0,0,0,0.12);
            box-shadow: 0 1px 2px rgba(0,0,0,0.05);
            display: inline-block;
            vertical-align: middle;
        }
        .display-value {
            min-width: 50px;
            text-align: center;
        }
    </style>
</head>
<body>
    <div class="controls">
        <!-- 字体大小控制 -->
        <div class="control-section">
            <span>字体大小调节</span>
            <div class="control-group">
                <button onclick="changeFontSize(-1)">A-</button>
                <span id="fontSizeDisplay" class="display-value">16px</span>
                <button onclick="changeFontSize(1)">A+</button>
            </div>
        </div>
        
        <!-- 行距控制 -->
        <div class="control-section">
            <span>行距调节</span>
            <div class="control-group">
                <button onclick="changeLineHeight(-0.2)">行距-</button>
                <span id="lineHeightDisplay" class="display-value">1.6</span>
                <button onclick="changeLineHeight(0.2)">行距+</button>
            </div>
        </div>
        
        <!-- 字体颜色控制 -->
        <div class="control-section">
            <span>字体颜色选择</span>
            <div class="control-group">
                <select id="textColorSelect" aria-label="字体颜色选择">
                    <option value="#111111">黑色 (#111111)</option>
                    <option value="#2F4F4F">深石板灰（护眼）(#2F4F4F)</option>
                    <option value="#333333" selected>默认深灰 (#333333)</option>
                    <option value="#444444">中灰 (#444444)</option>
                    <option value="#5B4636">温暖棕（护眼）(#5B4636)</option>
                    <option value="#0066cc">深蓝 (#0066cc)</option>
                    <option value="#006600">深绿（护眼）(#006600)</option>
                    <option value="#8a2be2">紫色 (#8a2be2)</option>
                    <option value="#6B4423">柔和棕（护眼）(#6B4423)</option>
                    <option value="#4A4A4A">柔和深灰 (#4A4A4A)</option>
                </select>
                <span id="textColorPreview" class="color-preview" style="background:#333"></span>
            </div>
        </div>
        
        <!-- 背景颜色控制（中间/左侧/右侧） -->
        <div class="control-section">
            <span>背景颜色选择</span>
            <div style="display:flex;flex-direction:column;gap:8px;">
                <div class="control-group">
                    <span>中间背景</span>
                    <select id="centerColorSelect" aria-label="中间背景颜色选择">
                        <option value="#ffffff" selected>白色 (#ffffff)</option>
                        <option value="#fffdf0">暖白/米色 (#fffdf0)</option>
                        <option value="#fffbe6">柔和乳白 (#fffbe6)</option>
                        <option value="#ffffee">浅黄 (#ffffee)</option>
                        <option value="#f7fff7">护眼绿（浅）(#f7fff7)</option>
                        <option value="#f6f9ff">护眼蓝（浅）(#f6f9ff)</option>
                    </select>
                    <span id="centerColorPreview" class="color-preview" style="background:#ffffff;margin-left:8px"></span>
                </div>
                <div class="control-group">
                    <span>左侧背景</span>
                    <select id="leftColorSelect" aria-label="左侧背景颜色选择">
                        <option value="#f5f5f5" selected>浅灰 (#f5f5f5)</option>
                        <option value="#ffffff">白色 (#ffffff)</option>
                        <option value="#fffdf0">暖白/米色（护眼）(#fffdf0)</option>
                        <option value="#fffbe6">柔和乳白（护眼）(#fffbe6)</option>
                        <option value="#ffffee">浅黄（护眼）(#ffffee)</option>
                        <option value="#f7fff7">护眼绿（浅）(#f7fff7)</option>
                        <option value="#f0fff0">浅绿 (#f0fff0)</option>
                        <option value="#f6f9ff">护眼蓝（浅）(#f6f9ff)</option>
                        <option value="#f7f0ff">浅紫 (#f7f0ff)</option>
                        <option value="#eeeae0">米灰 (#eeeae0)</option>
                    </select>
                    <span id="leftColorPreview" class="color-preview" style="background:#f5f5f5;margin-left:8px"></span>
                </div>
                <div class="control-group">
                    <span>右侧背景</span>
                    <select id="rightColorSelect" aria-label="右侧背景颜色选择">
                        <option value="#f5f5f5" selected>浅灰 (#f5f5f5)</option>
                        <option value="#ffffff">白色 (#ffffff)</option>
                        <option value="#fffdf0">暖白/米色（护眼）(#fffdf0)</option>
                        <option value="#fffbe6">柔和乳白（护眼）(#fffbe6)</option>
                        <option value="#ffffee">浅黄（护眼）(#ffffee)</option>
                        <option value="#f7fff7">护眼绿（浅）(#f7fff7)</option>
                        <option value="#f0fff0">浅绿 (#f0fff0)</option>
                        <option value="#f6f9ff">护眼蓝（浅）(#f6f9ff)</option>
                        <option value="#f7f0ff">浅紫 (#f7f0ff)</option>
                        <option value="#eeeae0">米灰 (#eeeae0)</option>
                    </select>
                    <span id="rightColorPreview" class="color-preview" style="background:#f5f5f5;margin-left:8px"></span>
                </div>
            </div>
        </div>
        
        <!-- 分页信息 -->
        <div class="chunk-info">
            第 {{.CurrentChunk}} / {{.TotalChunks}} 部分
        </div>
    </div>
    
    <div class="page-center">
        <div class="content" id="mainContent">
            {{.Content}}
        </div>
    </div>

    <script>
        // 确保DOM加载完成后执行
        document.addEventListener('DOMContentLoaded', function() {
            // 获取元素引用
            const contentElement = document.getElementById('mainContent');
            let currentFontSize = 16;
            let currentLineHeight = 1.6; // 默认行距
            
            // 字体颜色切换功能
            document.querySelectorAll('#textColorOptions .color-option').forEach(option => {
                option.addEventListener('click', function() {
                    document.querySelectorAll('#textColorOptions .color-option').forEach(o => o.classList.remove('active'));
                    this.classList.add('active');
                    const color = this.getAttribute('data-color');
                    contentElement.style.color = color;
                });
            });
            
            // 中央内容背景颜色切换功能（下拉菜单）
            const centerColorSelect = document.getElementById('centerColorSelect');
            const centerColorPreview = document.getElementById('centerColorPreview');
            centerColorSelect.addEventListener('change', function() {
                const c = this.value;
                document.documentElement.style.setProperty('--center-bg', c);
                contentElement.style.backgroundColor = c;
                centerColorPreview.style.background = c;
            });

            // 左侧/右侧：使用下拉菜单选择颜色，更新 CSS 变量与预览
            const leftColorSelect = document.getElementById('leftColorSelect');
            const rightColorSelect = document.getElementById('rightColorSelect');
            const leftPreview = document.getElementById('leftColorPreview');
            const rightPreview = document.getElementById('rightColorPreview');

            leftColorSelect.addEventListener('change', function() {
                const c = this.value;
                document.documentElement.style.setProperty('--left-bg', c);
                leftPreview.style.background = c;
            });
            rightColorSelect.addEventListener('change', function() {
                const c = this.value;
                document.documentElement.style.setProperty('--right-bg', c);
                rightPreview.style.background = c;
            });

            // 字体颜色选择（下拉菜单，10色）
            const textColorSelect = document.getElementById('textColorSelect');
            const textColorPreview = document.getElementById('textColorPreview');
            textColorSelect.addEventListener('change', function() {
                const c = this.value;
                contentElement.style.color = c;
                textColorPreview.style.background = c;
            });
            
            // 字体大小调节功能
            window.changeFontSize = function(change) {
                currentFontSize += change;
                // 限制字体大小范围
                if (currentFontSize < 10) currentFontSize = 10;
                if (currentFontSize > 36) currentFontSize = 36;
                
                contentElement.style.fontSize = currentFontSize + "px";
                document.getElementById("fontSizeDisplay").textContent = currentFontSize + "px";
            };
            
            // 行距调节功能
            window.changeLineHeight = function(change) {
                currentLineHeight += change;
                // 限制行距范围（0.8到3.0之间）
                if (currentLineHeight < 0.8) currentLineHeight = 0.8;
                if (currentLineHeight > 3.0) currentLineHeight = 3.0;
                
                // 保留一位小数显示
                const displayValue = currentLineHeight.toFixed(1);
                contentElement.style.lineHeight = currentLineHeight;
                document.getElementById("lineHeightDisplay").textContent = displayValue;
            };
        });
    </script>
</body>
</html>`

// 计算HTML模板的基础大小（不含内容）
func getBaseHTMLSize(fileName string, totalChunks, currentChunk int) int {
	data := TemplateData{
		Content:      "",
		FileName:     fileName,
		TotalChunks:  totalChunks,
		CurrentChunk: currentChunk,
	}
	tmpl, _ := template.New("htmlTemplate").Parse(htmlTemplate)
	var buf io.Writer = &bytes.Buffer{}
	tmpl.Execute(buf, data)
	return buf.(*bytes.Buffer).Len()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: go run txt2html.go <文件名> [编码]")
		fmt.Println("示例: go run txt2html.go document.txt gbk")
		return
	}

	inputFilePath := os.Args[1]
	encodingName := "utf-8"
	if len(os.Args) > 2 {
		encodingName = os.Args[2]
	}

	if _, err := os.Stat(inputFilePath); os.IsNotExist(err) {
		fmt.Printf("错误: 文件不存在 - %s\n", inputFilePath)
		return
	}

	inputFile, err := os.Open(inputFilePath)
	if err != nil {
		fmt.Printf("无法打开文件: %v\n", err)
		return
	}
	defer inputFile.Close()

	// 删除旧的输出目录（确保生成新文件）
	outputDir := filepath.Base(inputFilePath) + "_html_chunks"
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	fileInfo, _ := inputFile.Stat()
	fmt.Printf("处理文件: %s (%.2f MB)\n", inputFile.Name(), float64(fileInfo.Size())/1024/1024)

	decoder := getEncodingDecoder(encodingName)
	if decoder == nil {
		fmt.Printf("不支持的编码: %s\n", encodingName)
		return
	}

	reader := transform.NewReader(inputFile, decoder.NewDecoder())
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, readBufferSize), readBufferSize)

	// 先预读计算总块数（粗略估计）
	var totalLines int
	tempReader := transform.NewReader(inputFile, decoder.NewDecoder())
	tempScanner := bufio.NewScanner(tempReader)
	for tempScanner.Scan() {
		totalLines++
	}
	inputFile.Seek(0, 0) // 重置文件指针

	// 估算总块数
	estimatedTotalChunks := (totalLines * 100) / 30000 // 估算值，实际会动态调整
	if estimatedTotalChunks < 1 {
		estimatedTotalChunks = 1
	}

	baseHTMLSize := getBaseHTMLSize(filepath.Base(inputFilePath), estimatedTotalChunks, 1)
	remainingSize := targetHTMLSize - baseHTMLSize
	if remainingSize < 0 {
		remainingSize = 1024 // 确保至少能容纳一些内容
	}

	var currentContent string
	var chunkNumber int = 1
	var allChunks []string

	// 读取内容并按HTML大小分割
	for scanner.Scan() {
		line := scanner.Text() + "\n"
		escapedLine := template.HTMLEscapeString(line)
		lineSize := len(escapedLine)

		// 如果添加当前行会超过目标大小，则生成新文件
		if len(currentContent)+lineSize > remainingSize {
			allChunks = append(allChunks, currentContent)
			currentContent = escapedLine
			chunkNumber++
			remainingSize = targetHTMLSize - getBaseHTMLSize(filepath.Base(inputFilePath), estimatedTotalChunks, chunkNumber)
			if remainingSize < 0 {
				remainingSize = 1024
			}
		} else {
			currentContent += escapedLine
		}
	}

	// 添加最后一块内容
	if currentContent != "" {
		allChunks = append(allChunks, currentContent)
	}

	// 修正总块数
	actualTotalChunks := len(allChunks)

	// 生成所有HTML文件
	for i, content := range allChunks {
		fileName := fmt.Sprintf("%s_chunk_%d.html",
			filepath.Base(inputFilePath[:len(inputFilePath)-len(filepath.Ext(inputFilePath))]),
			i+1)
		outputPath := filepath.Join(outputDir, fileName)

		data := TemplateData{
			Content:      content,
			FileName:     filepath.Base(inputFilePath),
			TotalChunks:  actualTotalChunks,
			CurrentChunk: i + 1,
		}

		generateHTML(outputPath, data)
		fmt.Printf("已生成: %s (约 %.2f KB)\n", outputPath, float64(getFileSize(outputPath))/1024)
	}

	fmt.Printf("处理完成! 共生成 %d 个文件，保存到 %s\n", actualTotalChunks, outputDir)
}

func getEncodingDecoder(encodingName string) encoding.Encoding {
	switch encodingName {
	case "utf-8", "utf8":
		return unicode.UTF8
	case "utf-16", "utf16":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	case "utf-16be":
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
	case "utf-16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	case "gbk", "ansi":
		return simplifiedchinese.GBK
	default:
		return nil
	}
}

func generateHTML(outputPath string, data TemplateData) error {
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	tmpl, err := template.New("htmlTemplate").Parse(htmlTemplate)
	if err != nil {
		return err
	}

	return tmpl.Execute(outputFile, data)
}

func getFileSize(path string) int64 {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fileInfo.Size()
}
