package input

import (
	"log"
	"sync"

	"github.com/bendahl/uinput"
)

var (
	mouse     uinput.Mouse
	mouseOnce sync.Once
)

// InitMouse инициализирует виртуальную мышь
func InitMouse() error {
	var err error
	mouseOnce.Do(func() {
		// Создаем виртуальное устройство "HyprLink Mouse"
		mouse, err = uinput.CreateMouse("/dev/uinput", []byte("HyprLink Mouse"))
		if err != nil {
			log.Printf("Failed to create uinput device: %v. Make sure you have permissions (uinput group or root).", err)
		} else {
			log.Println("Virtual mouse initialized successfully")
		}
	})
	return err
}

// Move перемещает курсор на dx, dy
func Move(dx, dy int32) {
	if mouse != nil {
		mouse.Move(dx, dy)
	}
}

// Click выполняет нажатие. btn: "left", "right"
func Click(btn string) {
	if mouse != nil {
		switch btn {
		case "left":
			mouse.LeftClick()
		case "right":
			mouse.RightClick()
		}
	}
}

// Scroll выполняет прокрутку
func Scroll(dy int32) {
	// Библиотека может не поддерживать скролл из коробки в базовом интерфейсе,
	// но для начала хватит Move/Click. Если критично - допишем raw events.
}

func Close() {
	if mouse != nil {
		mouse.Close()
	}
}
