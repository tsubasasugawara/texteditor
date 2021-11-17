package main

import (
	"bufio"
	"github.com/nsf/termbox-go"
	"github.com/pkg/term/termios"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sys/unix"
	"io"
	"log"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
	"regexp"
)

// キー対応表
const (
	// size 3 Scroll
	ArrowUp	byte = 65
	ArrowDown	   = 66
	ArrowRight	  = 67
	ArrowLeft	   = 68

	// size 1
	Ctrlq	 = 17 //quit
	Ctrls	 = 19 //save
	Ctrlz	 = 26 //undo
	Ctrly	 = 25 //redo
	Ctrlk	 = 11 //up
	Ctrlj	 = 10 //down
	Ctrll	 = 12 //right
	Ctrlh	 = 8  //left
	Ctrlr	 = 18 //Delete Row
	Enter	 = 13
	BackSpace = 127
	Tab	   = 9
)


// ファイルデータ
type TString struct {
	data	[]string //ファイルの中身
	path	string   //ファイルのパス
	history []string //操作の履歴
}

var File TString

// エディター
type EditorConfig struct {
	defaultTtystate unix.Termios //初期のターミナル属性
	cursorx		 int		  //カーソルのx座標
	cursory		 int		  //カーソルのy座標
	wsRow		   int		  //行
	wsCol		   int		  //列
	drawingStartRow int		  //描画する最初の行
	drawingStartCol int		  //描画する最初の列
	reservedWords  []string
}
var Editor EditorConfig

// Editorクラスのコンストラクタ
func (ele *EditorConfig) construct() {
	ele.cursorx = 0
	ele.cursory = 0
	ele.drawingStartRow = 0
	ele.drawingStartCol = 0
	getWindowSize()
}

type ReservedWords struct {
	words [][]string
	/** 行数:色
	 * 0:blue
	 * 1:pink
	 * 2:green
	*/
}
var RWords ReservedWords

func (ele *ReservedWords) construct() {
	ele.words = [][]string{
		{"func", "type", "struct", "const", "var", "nil", "package", "import"},
		{ "for", "if", "else if", "else", "return", "defer"},
		{ "int", "string", "error"},
	}
}

// ウィンドウサイズを取得し、Editorに設定する
func getWindowSize() {
	var err error
	Editor.wsCol, Editor.wsRow, err = terminal.GetSize(syscall.Stdin)
	if err != nil {
		log.Fatal(err)
	}
}

// 大きい方を返す
func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

// 小さい方を返す
func min(a int, b int) int {
	if a > b {
		return b
	}
	return a
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

		data = strings.Replace(data+string(line), "\t", "	", -1) + "\n"
		if !isPrefix {
			File.data = append(File.data, data)
			data = ""
		}
	}
}

// ファイル書き込み
func toFile() {
	f, err := os.Create(File.path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	for i := 0; i < len(File.data); i++ {
		data := strings.Replace(File.data[i], "	", "\t", -1)
		_, er := f.WriteString(data)
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
	termios.Tcsetattr(uintptr(syscall.Stdin), termios.TCSANOW, attr)
}

// バッファの値を取得する
func readBuffer(bufCh chan []byte) {
	buf := make([]byte, 1024)
	for {
		if n, err := syscall.Read(syscall.Stdin, buf); err == nil {
			bufCh <- buf[:n]
		}
	}
}

// 文字列の入力を受け取る
func getChar() {
	bufCh := make(chan []byte, 1)
	go readBuffer(bufCh)

	running := true
	for running {
		p := <-bufCh
		switch len(p) {
		case 3:
			switch p[2] {
			case ArrowUp:
				moveCursor(0, -1)
			case ArrowDown:
				moveCursor(0, 1)
			case ArrowRight:
				moveCursor(1, 0)
			case ArrowLeft:
				moveCursor(-1, 0)
			}
		default:
			switch p[0] {
			case Enter:
				enter()
				moveCursor(0, 1)
			case BackSpace:
				backSpace()
			case Ctrlq:
				running = false
			case Ctrls:
				toFile()
			case Ctrlz:
			case Ctrly:
			case Ctrlr:
				deleteRow()
			case Ctrlk:
				moveCursor(0, -1)
			case Ctrlj:
				moveCursor(0, 1)
			case Ctrll:
				moveCursor(1, 0)
			case Ctrlh:
				moveCursor(-1, 0)
			case Tab:
				textInsertion("	")
				moveCursor(4, 0)
			default:
				textInsertion(*(*string)(unsafe.Pointer(&p)))
				moveCursor(1, 0)
			}
		}
		setText()
	}
}

// カーソル移動制御
func moveCursor(addx int, addy int) {
	canMoveCursor(addx, addy)
	termbox.SetCursor(Editor.cursorx, Editor.cursory)
	termbox.Flush()
}

// カーソル位置を移動する
func canMoveCursor(addx int, addy int) {
	X := Editor.cursorx + addx
	if X >= 0 && X < Editor.wsCol {
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

	checkX(Editor.cursory + Editor.drawingStartRow)
}

// 垂直移動したときに、現在の描画位置よりも
// 文字列が短かった場合に文字列の最後尾にカーソルを移動する
func checkX(r int) {
	if len(File.data) == 0 {
		Editor.cursorx = 0
		Editor.drawingStartCol = 0
		return
	}

	length := len(File.data[r]) - Editor.drawingStartCol
	if length <= 0 {
		Editor.cursorx = 0
		Editor.drawingStartCol = max(0, len(File.data[r])-1)
	}
	if length > 0 && length < Editor.wsCol && Editor.cursorx > length-1 {
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
	if X >= Editor.wsCol && len(File.data[Editor.cursory+Editor.drawingStartRow])-1-Editor.drawingStartCol >= Editor.wsCol {
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
	if Y >= Editor.wsRow && len(File.data)-1-Editor.drawingStartRow >= Editor.wsRow {
		Editor.cursory = Editor.wsRow - 1
		Editor.drawingStartRow++
	}
}

// テキストを表示する
func setText() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	for y := 0; y < Editor.wsRow; y++ {
		// もしファイルの行数が表示限界の行数よりも
		// 小さい時に"~"を表示する
		if y+Editor.drawingStartRow >= len(File.data) {
			if y == 0 {
				continue
			}

			termbox.SetCell(0, y, rune('~'), termbox.ColorDefault, termbox.ColorDefault)
		} else {
			text := File.data[y+Editor.drawingStartRow]
			runeText := []rune(text)

			colors := checkReservedWord(File.data[y+Editor.drawingStartRow])

			x := 0
			for j := Editor.drawingStartCol; j < min(Editor.drawingStartCol+Editor.wsCol, len(runeText)); j++ {
				fontColor := decideColor(colors[j])
				termbox.SetCell(x, y, runeText[j], fontColor, termbox.ColorDefault)

				x += utf8.RuneCountInString(string(text[j]))
			}
		}
	}
	termbox.Flush()
}

func decideColor(n int) termbox.Attribute {
	switch n {
	case 1:
		return termbox.ColorBlue
	case 2:
		return termbox.ColorLightMagenta
	case 3:
		return termbox.ColorLightGreen
	}
	// 0のとき
	return termbox.ColorDefault
}

// 予約語を確認
func checkReservedWord(s string) []int {
	res := make([]int, len(s))
	for row, V := range RWords.words {
	for _, v := range V {
		i := 0
		for {
			i= strings.Index(s[i:],v)
			if i == -1 {
				break
			}

			num := row+1
			r := `[a-z]|[A-Z]|[0-9]`
			if (i > 0 && check_regexp(r, string(s[i-1]))) || (i+len(v) < len(s)-1 && check_regexp(r, string(s[i+len(v)]))) || (i > 0 && i+len(v) < len(s)-1 && s[i-1] == '"' || s[i+len(v)] == '"') {
				num = 0
			}

			for j := i; j < i + len(v); j++ {
				res[j] = num
			}
			i += len(v)
		}
	}
}
	return res
}

func check_regexp(reg, str string) bool{
	return (regexp.MustCompile(reg).Match([]byte(str)))
}

// 文字を挿入する
func textInsertion(s string) {
	r := Editor.cursory + Editor.drawingStartRow
	c := Editor.cursorx + Editor.drawingStartCol

	if len(File.data) == 0 {
		File.data = append(File.data, s+"\n")
		return
	}

	length := len(File.data[r])
	if length == c && length >= 1 && File.data[r][length-1] == '\n' {
		File.data[r] = strings.Replace(File.data[r], "\n", s, -1) + "\n"
		return
	}

	File.data[r] = File.data[r][:c] + s + File.data[r][c:]
	insertNewLineCode(&File.data[r])
}

// enterを押したとき
func enter() {
	r := Editor.cursory + Editor.drawingStartRow
	c := Editor.cursorx + Editor.drawingStartCol

	if len(File.data) == 0 {
		File.data = append(File.data, "\n")
		return
	}

	File.data = append(File.data[:r+1], File.data[r:]...)
	File.data[r+1] = isTab(File.data[r][:c]) + File.data[r][c:]
	File.data[r] = File.data[r][:c]
	insertNewLineCode(&File.data[r+1])
	insertNewLineCode(&File.data[r])
}

// 前の行のタブを引き継ぐ
func isTab(s string) string {
	n := strings.Count(s, "	")
	t := ""
	for i := 0; i < n; i++ {
		t += "	"
	}
	return t
}

// 改行コードを挿入
func insertNewLineCode(s *string) {
	if !strings.Contains(*s, "\n") {
		*s = *s + "\n"
	}
}

// BackSpace
func backSpace() {
	r := Editor.cursory + Editor.drawingStartRow
	c := Editor.cursorx + Editor.drawingStartCol

	if c == 0 && r == 0 {
		return
	}

	if c == 0 && r > 0 {
		length := len(File.data[r-1])
		File.data[r-1] = strings.Replace(File.data[r-1], "\n", File.data[r], 1)
		File.data = append(File.data[:r], File.data[r+1:]...)
		moveCursor(length-1, -1)
		return
	}

	File.data[r] = File.data[r][:c-1] + File.data[r][c:]
	moveCursor(-1, 0)
}

// 一行削除する
func deleteRow() {
	r := Editor.cursory + Editor.drawingStartRow
	if r == 0 && len(File.data) <= 1 {
		File.data[r] = ""
		return
	}
	File.data = append(File.data[:r], File.data[r+1:]...)
	moveCursor(0, -1)
}

func main() {
	File.path = os.Args[1]
	fromFile()

	Editor.construct()
	RWords.construct()

	settingTermios()

	err := termbox.Init()
	if err != nil {
		log.Fatal(err)
	}
	defer termbox.Close()
	setText()

	getChar()

	resetRawMode(&Editor.defaultTtystate)
}
