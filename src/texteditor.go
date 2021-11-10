package main

import (
	"fmt"
	"os"
	"log"
	"syscall"
	"unsafe"
	"bufio"
	"io"
	"strings"
	"unicode/utf8"
	"golang.org/x/sys/unix"
	"golang.org/x/crypto/ssh/terminal"
	"github.com/pkg/term/termios"
	"github.com/nsf/termbox-go"
)

// キー対応表
const (
	ArrowUp byte= 65
	ArrowDown   = 66
	ArrowRight  = 67
	ArrowLeft   = 68
	Ctrlq				= 17
	Enter 			= 13
	BackSpace		= 127
	Tab					= 9
)

// ファイルデータ
type TString struct {
	data []string				//ファイルの中身
	path string				//ファイルのパス
	history []string	//操作の履歴
}
var File TString

// エディター
type EditorConfig struct {
	defaultTtystate unix.Termios //初期のターミナル属性
	cursorx int									 //カーソルのx座標
	cursory int									 //カーソルのy座標
	wsRow int										 //行
	wsCol int										 //列
	drawingStartRow int					 //描画する最初の行
	drawingStartCol int					 //描画する最初の列
}
// Editorクラスのコンストラクタ
func(ele *EditorConfig) construct() {
	ele.cursorx = 0
	ele.cursory = 0
	ele.drawingStartRow = 0
	ele.drawingStartCol = 0
	getWindowSize()
}
var Editor EditorConfig

// ウィンドウサイズを取得し、Editorに設定する
func getWindowSize() {
	var err error
	Editor.wsCol, Editor.wsRow, err = terminal.GetSize(syscall.Stdin)
	if err != nil {
		log.Fatal(err)
	}
}

// 大きい方を返す
func max(a int, b int) int{
	if a > b {
		return a
	} else {
		return b
	}
}

// 小さい方を返す
func min(a int, b int) int{
	if a > b {
		return b
	} else {
		return a
	}
}

// ファイルを読み込みファイルデータを返す
func fromFile() {
	f, err := os.OpenFile(File.path, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)

	// データを格納するスライス
	var data string
	for {
		line, isPrefix, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		data = data + string(line)
		data = strings.Replace(data, "\t", "    ", -1)
		if !isPrefix {
			File.data = append(File.data, data)
			data = ""
		}
	}
}

// ファイル書き込み
func toFile() {
	f, err := os.OpenFile(File.path, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	for i := 0; i < len(File.data); i++ {
		data := strings.Replace(File.data[i], "    ", "\t", -1)
		_, er := f.WriteString(data + "\n")
		if er != nil {
			log.Fatal(er)
		}
	}
}

// 起動時のtermiosの設定
func settingTermios() {
	termios.Tcgetattr(uintptr(syscall.Stdin), &Editor.defaultTtystate)
	ttystate := Editor.defaultTtystate
	setRawMode(&ttystate)
}

// 非カノニカルモードに設定する
func setRawMode(attr *unix.Termios) {
	attr.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	attr.Cflag &^= syscall.CSIZE | syscall.PARENB
	attr.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN
	attr.Cc[syscall.VMIN] = 1
	attr.Cc[syscall.VTIME] = 0
	termios.Tcsetattr(uintptr(syscall.Stdin), termios.TCSANOW, attr)
}

// ターミナル属性をリセットする
func resetRawMode(attr *unix.Termios) {
	termios.Tcsetattr(uintptr(syscall.Stdin),termios.TCSANOW,attr)
}

// バッファの値を取得する
func readBuffer(bufCh chan []byte) {
	buf := make([]byte, 1024)
	for{
		if n,err := syscall.Read(syscall.Stdin,buf); err == nil {
			bufCh <- buf[:n]
		}
	}
}

// 文字列の入力を受け取る
func inputChar() {
	bufCh := make(chan []byte, 1)
	go readBuffer(bufCh)

	running := true

	for running{
		p := <-bufCh
		// fmt.Println(p) 中身確認用
		switch len(p){
		case 3:
			switch p[2]{
			case ArrowUp:
				moveCursor(0,-1)
			case ArrowDown:
				moveCursor(0,1)
			case ArrowRight:
				moveCursor(1,0)
			case ArrowLeft:
				moveCursor(-1,0)
			}
		}
		default:
			switch p[0] {
			case Enter:
				fmt.Println("enter")
			case BackSpace:
				fmt.Println("backspace")
			case Ctrlq:
				running = false
			default:
				t := *(*string)(unsafe.Pointer(&p))
				fmt.Printf(t)
			}
		}
	}
}

// カーソル移動制御
func moveCursor(addx int, addy int) {
	canMoveCursor(addx, addy)
	setText()
	termbox.SetCursor(Editor.cursorx,Editor.cursory)
	termbox.Flush()
}

// カーソル位置を移動する
func canMoveCursor(addx int, addy int) {
	r := Editor.cursory + Editor.drawingStartRow //行番号
	c := Editor.cursorx + Editor.drawingStartCol //列番号

	X := Editor.cursorx
	// 現在の行に1つ以上文字があるとき、文字のサイズを考慮
	if len(File.data[r]) > 0{
			X += addx * utf8.RuneCountInString(string(File.data[r][c]))
	}

	if X >= 0 && X <= Editor.wsCol{
		Editor.cursorx = X
	} else {
		controlHorizontalMovement(X)
	}

	Y := Editor.cursory + addy
	if Y >= 0 && Y < Editor.wsRow && len(File.data) > Y {
		Editor.cursory = Y
	} else {
		controlVerticalMovement(Y)
	}

	// 移動後に求めなおす
	r = Editor.cursory + Editor.drawingStartRow

	// 垂直移動したときに、現在の描画位置よりも
	// 文字列が短かった場合に文字列の最後尾に移動する
	length := len(File.data[r]) - Editor.drawingStartCol;
	if length <= 0 {
		Editor.cursorx = 0
		temp := len(File.data[r]) - 1
		Editor.drawingStartCol = max(0,temp)
	}
	if length > 0 && length < Editor.wsCol && Editor.cursorx > length - 1 {
		Editor.cursorx = length - 1
	}
}

// 水平移動を管理
func controlHorizontalMovement(X int) {
	// 左スクロール
	if X < 0 && Editor.drawingStartCol > 0 {
		Editor.cursorx = 0
		Editor.drawingStartCol--
	}
	// 右スクロール
	if X > Editor.wsCol && len(File.data[Editor.cursory]) - 1 - Editor.drawingStartCol > Editor.wsCol{
		Editor.cursorx = Editor.wsCol
		Editor.drawingStartCol++
	}
}

// 垂直移動を管理
func controlVerticalMovement(Y int) {
	// 上スクロール
	if Y < 0 && Editor.drawingStartRow > 0 {
		Editor.cursory = 0
		Editor.drawingStartRow--
	}
	// 下スクロール
	if Y >= Editor.wsRow && (len(File.data) - 1) - Editor.drawingStartRow > Editor.wsRow {
		Editor.cursory = Editor.wsRow
		Editor.drawingStartRow++
	}
}

// テキストを表示する
func setText() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	for y := 0; y < Editor.wsRow; y++ {
		// もしファイルの行数が表示限界の行数よりも
		// 小さい時に"~"を表示する
		if y + Editor.drawingStartRow >= len(File.data) {
			termbox.SetCell(0,y,rune('~'),termbox.ColorDefault,termbox.ColorDefault)
		} else {
			text := File.data[y + Editor.drawingStartRow]
			runeText := []rune(text)

			x := 0
			for j := Editor.drawingStartCol; j < len(runeText); j++ {
				w := utf8.RuneCountInString(string(text[j]))

				// 行の末まで来た時、または、表示限界の列数を超えたら
				// 次の行の描画をする
				if x + w > Editor.wsCol  || text[j] == '\n' {
					break
				}

				termbox.SetCell(x,y,runeText[j],termbox.ColorDefault, termbox.ColorDefault)

				x += w
			}
		}
	}
	termbox.Flush()
}


func main(){
	File.path = os.Args[1]
	fromFile()

	Editor.construct()

	settingTermios()

	err := termbox.Init()
	if err != nil {
		log.Fatal(err)
	}
	defer termbox.Close()
	setText()

	inputChar()

	resetRawMode(&Editor.defaultTtystate)
}