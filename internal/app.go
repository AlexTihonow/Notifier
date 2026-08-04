package notifier

import (
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"my-mailer/internal/config"
)

const (
	batchSize    = 10
	pollInterval = 5 * time.Second
	workerCount  = 5
)

func Run (conf config.Config) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)


	db, err := OpenDB(conf)
	if err != nil {
		slog.Error("не удалось подключиться к базе", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	stop := make(chan struct{})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		slog.Info("получен сигнал остановки, завершаем текущую пачку")
		close(stop)
	}()

	slog.Info("сервис запущен",
		"batch_size", batchSize,
		"poll_interval", pollInterval.String(),
		"workers", workerCount,
	)
	runWorker(db, conf, stop)
	slog.Info("сервис остановлен")
}

func runWorker(db *sql.DB, cfg config.Config, stop chan struct{}) {
	count, err := resetWorking(db)
	if err != nil {
		slog.Error("не удалось вернуть зависшие записи", "err", err)
	} else if count > 0 {
		slog.Info("возвращено в очередь записей", "count", count)
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pollLoop(db, cfg, stop, id)
		}(i)
	}
	wg.Wait()
}

func pollLoop(db *sql.DB, cfg config.Config, stop chan struct{}, id int) {
	log := slog.With("worker", id)
	
	idle := false

	for {
		did, err := processBatch(db, cfg, log)

		if did {
			idle = false

			select {
			case <-stop:
				return
			default:
				continue
			}
		}

		if !idle {
			if err != nil {
				log.Error("не удалось получить пачку", "err", err)
			} else {
				log.Info("очередь пуста, ждём следующий опрос", "poll_interval", pollInterval.String())
			}
			idle = true
		}
		select {
		case <-stop:
			return
		case <-time.After(pollInterval):
		}
	}
}

func processBatch(db *sql.DB, cfg config.Config, log *slog.Logger) (bool, error) {
	batch, err := takePendingBatch(db, batchSize)
	if err != nil {
		return false, err
	}
	if len(batch) == 0 {
		return false, nil
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
			log.Warn("неизвестный target", "id", m.ID, "target", m.Target)
			setStatus(db, m.ID, false, log)
		}
	}

	log.Info("взята пачка",
		"total", len(batch),
		"emails", len(emails),
		"pushes", len(pushes),
	)

	var wg sync.WaitGroup

	for _, m := range emails {
		wg.Add(1)
		go func(m Mailing) {
			defer wg.Done()
			processEmail(db, cfg, m, log)
		}(m)
	}

	for _, m := range pushes {
		wg.Add(1)
		go func(m Mailing) {
			defer wg.Done()
			processPush(db, cfg, m, log)
		}(m)
	}

	wg.Wait()
	return true, nil
}

func processEmail(db *sql.DB, cfg config.Config, m Mailing, log *slog.Logger) {
	recipients := splitRecipients(m.Recipients)
	if len(recipients) == 0 {
		log.Warn("нет подходящих адресов")
		setStatus(db, m.ID, false, log)
		return
	}

	sent := 0
	var failed []string

	for _, address := range recipients {
		if err := send(cfg, address, m.Subject, m.Body); err != nil {
			log.Error("не удалось отправить письмо", "to", address, "err", err)
			failed = append(failed, address)
			continue
		}
		sent++
		log.Info("письмо отправлено", "to", address, "subject", m.Subject)
	}

	if sent == 0 {
		log.Error("письма не ушли никому", "recipients", len(recipients))
		setStatus(db, m.ID, false, log)
		return
	}

	log.Info("email-запись обработана",
		"subject", m.Subject,
		"sent", sent,
		"failed", len(failed),
		"failed_addresses", failed,
	)

	setStatus(db, m.ID, true, log)
}

func processPush(db *sql.DB, cfg config.Config, m Mailing, log *slog.Logger) {
	sender := newPushSender(cfg)

	recipients := splitRecipients(m.Recipients)
	if len(recipients) == 0 {
		log.Warn("пустой список получателей")
		setStatus(db, m.ID, false, log)
		return
	}

	err := sender.send(recipients, m.Subject, m.Body)
	if err != nil {
		log.Error("не удалось отправить push", "recipients", len(recipients), "err", err)
		setStatus(db, m.ID, false, log)
		return
	}

	log.Info("push отправлен",
		"subject", m.Subject,
		"recipients", len(recipients),
		"recipients_list", recipients,
	)
	setStatus(db, m.ID, true, log)
}

func setStatus(db *sql.DB, id string, ok bool, log *slog.Logger) {
	var err error
	if ok {
		err = markFinished(db, id)
	} else {
		err = markFailed(db, id)
	}

	if err != nil {
		slog.Error("не удалось обновить статус", "id", id, "ok", ok, "err", err)
	}
}