package input

import (
	"log"
	"sync"

	"github.com/bendahl/uinput"
)

var (
	keyboard     uinput.Keyboard
	keyboardOnce sync.Once

	// Карта символов для QWERTY раскладки
	charMap = map[rune]int{
		'a': uinput.KeyA, 'b': uinput.KeyB, 'c': uinput.KeyC, 'd': uinput.KeyD,
		'e': uinput.KeyE, 'f': uinput.KeyF, 'g': uinput.KeyG, 'h': uinput.KeyH,
		'i': uinput.KeyI, 'j': uinput.KeyJ, 'k': uinput.KeyK, 'l': uinput.KeyL,
		'm': uinput.KeyM, 'n': uinput.KeyN, 'o': uinput.KeyO, 'p': uinput.KeyP,
		'q': uinput.KeyQ, 'r': uinput.KeyR, 's': uinput.KeyS, 't': uinput.KeyT,
		'u': uinput.KeyU, 'v': uinput.KeyV, 'w': uinput.KeyW, 'x': uinput.KeyX,
		'y': uinput.KeyY, 'z': uinput.KeyZ,

		'1': uinput.Key1, '2': uinput.Key2, '3': uinput.Key3, '4': uinput.Key4,
		'5': uinput.Key5, '6': uinput.Key6, '7': uinput.Key7, '8': uinput.Key8,
		'9': uinput.Key9, '0': uinput.Key0,

		' ': uinput.KeySpace, '\n': uinput.KeyEnter, '\b': uinput.KeyBackspace,
		'\t': uinput.KeyTab,

		'-': uinput.KeyMinus, '=': uinput.KeyEqual,
		'[': uinput.KeyLeftbrace, ']': uinput.KeyRightbrace,
		';': uinput.KeySemicolon, '\'': uinput.KeyApostrophe,
		'\\': uinput.KeyBackslash, ',': uinput.KeyComma,
		'.': uinput.KeyDot, '/': uinput.KeySlash,
		'`': uinput.KeyGrave,
	}

	// Символы, требующие Shift
	shiftMap = map[rune]int{
		'A': uinput.KeyA, 'B': uinput.KeyB, 'C': uinput.KeyC, 'D': uinput.KeyD,
		'E': uinput.KeyE, 'F': uinput.KeyF, 'G': uinput.KeyG, 'H': uinput.KeyH,
		'I': uinput.KeyI, 'J': uinput.KeyJ, 'K': uinput.KeyK, 'L': uinput.KeyL,
		'M': uinput.KeyM, 'N': uinput.KeyN, 'O': uinput.KeyO, 'P': uinput.KeyP,
		'Q': uinput.KeyQ, 'R': uinput.KeyR, 'S': uinput.KeyS, 'T': uinput.KeyT,
		'U': uinput.KeyU, 'V': uinput.KeyV, 'W': uinput.KeyW, 'X': uinput.KeyX,
		'Y': uinput.KeyY, 'Z': uinput.KeyZ,

		'!': uinput.Key1, '@': uinput.Key2, '#': uinput.Key3, '$': uinput.Key4,
		'%': uinput.Key5, '^': uinput.Key6, '&': uinput.Key7, '*': uinput.Key8,
		'(': uinput.Key9, ')': uinput.Key0,

		'_': uinput.KeyMinus, '+': uinput.KeyEqual,
		'{': uinput.KeyLeftbrace, '}': uinput.KeyRightbrace,
		':': uinput.KeySemicolon, '"': uinput.KeyApostrophe,
		'|': uinput.KeyBackslash, '<': uinput.KeyComma,
		'>': uinput.KeyDot, '?': uinput.KeySlash,
		'~': uinput.KeyGrave,
	}
)

func InitKeyboard() error {
	var err error
	keyboardOnce.Do(func() {
		// Используем uinput.CreateKeyboard, который регистрирует все основные клавиши
		keyboard, err = uinput.CreateKeyboard("/dev/uinput", []byte("HyprLink Keyboard"))
		if err != nil {
			log.Printf("Failed to create uinput keyboard: %v", err)
		} else {
			log.Println("Virtual keyboard initialized")
		}
	})
	return err
}

func PressKey(key int) {
	if keyboard != nil {
		keyboard.KeyPress(key)
	}
}

func TypeChar(char rune) {
	if keyboard == nil {
		return
	}

	// 1. Проверяем обычные символы (без Shift)
	if key, ok := charMap[char]; ok {
		keyboard.KeyPress(key)
		return
	}

	// 2. Проверяем символы с Shift
	if key, ok := shiftMap[char]; ok {
		keyboard.KeyDown(uinput.KeyLeftshift)
		keyboard.KeyPress(key)
		keyboard.KeyUp(uinput.KeyLeftshift)
		return
	}

	// Если символ не найден (например кириллица), ничего не делаем,
	// так как uinput эмулирует нажатие физических кнопок, а не ввод юникода.
	// Для поддержки кириллицы сервер должен знать текущую раскладку ОС,
	// что сложно реализовать надежно без X11/Wayland API.
}

func CloseKeyboard() {
	if keyboard != nil {
		keyboard.Close()
	}
}
