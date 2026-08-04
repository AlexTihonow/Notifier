package notifier

import (
	"database/sql"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"my-mailer/internal/configs"
)

const (
	batchSize    = 10
	pollInterval = 5 * time.Second
	workerCount  = 5
)

func Run (conf configs.Config){
	db, err := OpenDB(conf)
	if err != nil {
		log.Fatal("не удалось подключиться к базе")
	}
	defer db.Close()

	stop := make(chan struct{})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		log.Println("получен сигнал остановки, завершаем текущую пачку")
		close(stop)
	}()

	log.Printf("сервис запущен, размер пачки %d, опрос раз в %s", batchSize, pollInterval)
	runWorker(db, conf, stop)
	log.Println("сервис остановлен")
}

func runWorker(db *sql.DB, cfg configs.Config, stop chan struct{}) {
	count, err := resetWorking(db)
	if err != nil {
		log.Printf("не удалось вернуть зависшие записи: %v", err)
	} else if count > 0 {
		log.Printf("возвращено в очередь записей: %d", count)
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pollLoop(db, cfg, stop)
		}(i)
	}
	wg.Wait()
}

func pollLoop(db *sql.DB, cfg configs.Config, stop chan struct{}) {
	for {
		did := processBatch(db, cfg)

		if did {
			select {
			case <-stop:
				return
			default:
				continue
			}
		}

		select {
		case <-stop:
			return
		case <-time.After(pollInterval):
		}
	}
}

func processBatch(db *sql.DB, cfg configs.Config) bool {
	batch, err := takePendingBatch(db, batchSize)
	if err != nil {
		log.Printf("не удалось получить пачку уведомлений: %v", err)
		return false
	}
	if len(batch) == 0 {
		return false
	}

	var emails []Mailing
	var pushes []Mailing

	for _, m := range batch {
		switch m.Target {
		case "email":
			emails = append(emails, m)
		case "rudn_id":
			pushes = append(pushes, m)
		default:
			log.Printf("запись %s: неизвестный target %q", m.ID, m.Target)
			setStatus(db, m.ID, false)
		}
	}

	var wg sync.WaitGroup

	for _, m := range emails {
		wg.Add(1)
		go func(m Mailing) {
			defer wg.Done()
			processEmail(db, cfg, m)
		}(m)
	}

	for _, m := range pushes {
		wg.Add(1)
		go func(m Mailing) {
			defer wg.Done()
			processPush(db, cfg, m)
		}(m)
	}

	wg.Wait()
	return true
}

func processEmail(db *sql.DB, cfg configs.Config, m Mailing) {
	recipients := splitRecipients(m.Recipients)
	if len(recipients) == 0 {
		log.Printf("запись %s: не осталось ни одного подходящего адреса", m.ID)
		setStatus(db, m.ID, false)
		return
	}

	sent := 0
	var failed []string

	for _, address := range recipients {
		if err := send(cfg, address, m.Subject, m.Body); err != nil {
			log.Printf("запись %s, адрес %s: %v", m.ID, address, err)
			failed = append(failed, address)
			continue
		}
		sent++
	}

	if sent == 0 {
		log.Printf("запись %s: письма не ушли никому", m.ID)
		setStatus(db, m.ID, false)
		return
	}

	if len(failed) > 0 {
		log.Printf("запись %s: отправлено %d, не удалось %d (%v)", m.ID, sent, len(failed), failed)
	}
	setStatus(db, m.ID, true)
}

func processPush(db *sql.DB, cfg configs.Config, m Mailing) {
	sender := newPushSender(cfg)

	recipients := splitRecipients(m.Recipients)
	if len(recipients) == 0 {
		log.Printf("запись %s: пустой список получателей", m.ID)
		setStatus(db, m.ID, false)
		return
	}

	err := sender.send(recipients, m.Subject, m.Body)
	if err != nil {
		log.Printf("запись %s: %v", m.ID, err)
		setStatus(db, m.ID, false)
		return
	}

	setStatus(db, m.ID, true)
}

func setStatus(db *sql.DB, id string, ok bool) {
	var err error
	if ok {
		err = markFinished(db, id)
	} else {
		err = markFailed(db, id)
	}

	if err != nil {
		log.Printf("запись %s: не удалось обновить статус: %v", id, err)
	}
}