/*
 * mastercoderk@gmail.com
 */

package botservice

import (
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"chloe/ai"
	"chloe/def"
	"chloe/im"
	"chloe/util"

	log "github.com/jeanphorn/log4go"
)

var puncs = []string{",", ".", "，", "。", "!", "?", "！", "？"}

type BotTalkService struct {
	bot            def.MessageBot
	talkFact       def.ConversationFactory
	speechToText   def.SpeechToText
	imageGenerator def.ImageGenerator
	config         ai.AIConfig
	loop           bool
}

func NewTgBotService(tgbotToken string, aicfg ai.AIConfig) def.BotService {
	bot, err := im.NewTelegramBot(tgbotToken)
	if err != nil {
		log.Error("failed to start telegram bot %v", err)
	}
	return &BotTalkService{
		bot:            bot,
		talkFact:       ai.NewTalkFactory(aicfg),
		speechToText:   ai.NewSpeech2Text(aicfg.ApiKey),
		imageGenerator: ai.NewImageGenerator(aicfg.ApiKey),
		config:         aicfg,
		loop:           true,
	}
}

func (s *BotTalkService) Run() {
	sigchnl := make(chan os.Signal, 1)
	signal.Notify(sigchnl)
	go func() {
		for {
			sig := <-sigchnl
			s.handleSignal(sig)
		}
	}()
	for m := range s.bot.GetMessages() {
		func() {
			defer func() { _ = recover() }()
			chat := m.GetChat()
			memberCnt := chat.GetMemberCount()
			botUsername := chat.GetSelf().GetUserName()

			var text string
			var err error

			//
			voice, cleaner := m.GetVoice()
			if voice != "" {
				defer cleaner()
				// get text from voice
				if strings.EqualFold(filepath.Ext(voice), ".oga") {
					voice, cleaner = util.ConvertToMp3(voice)
					defer cleaner()
				}
				text, err = s.speechToText.Convert(voice)
				log.Debug("voice converted to text as: %s", text)
				if err != nil {
					log.Warn("speech to text failed, %v", err)
				}
			} else if m.GetText() != "" {
				text = m.GetText()
			}

			if strings.HasPrefix(text, "/draw ") {
				// draw image
				img, cleaner, err := s.imageGenerator.Generate(text[len("/draw "):])
				if err != nil {
					chat.ReplyMessage(err.Error(), m.GetID())
					return
				}
				defer cleaner()
				chat.ReplyImage(img, m.GetID())
			} else {
				if memberCnt > 2 && !s.isMentioned(text, botUsername) {
					return
				}

				log.Info("received question from %s: %s", m.GetUser().GetUserName(), text)
				talk := s.talkFact.GetTalk(chat.GetID())
				answer := talk.Ask(text)

				chat.ReplyMessage(answer, m.GetID())
				log.Info("replied to %s", m.GetUser().GetUserName())
			}
		}()
		if !s.loop {
			log.Info("stop running loop")
			time.Sleep(time.Duration(time.Second * 3))
			break
		}
	}
}

func (s *BotTalkService) isMentioned(text, botUsername string) bool {
	tokens := strings.Fields(text)
	if ssContain(tokens, "@"+botUsername, false) {
		return true
	}

	tokens = spliteByPunctuation(tokens)
	var head []string
	if len(tokens) >= 3 {
		head = tokens[:3]
	} else {
		head = tokens
	}

	if ssContain(head, s.config.BotName, true) {
		return true
	}

	return false
}

func ssContain(ss []string, target string, ignoreCase bool) bool {
	for _, s := range ss {
		if target == removePunctuation(s) ||
			(ignoreCase && strings.EqualFold(target, removePunctuation(s))) {
			return true
		}
	}
	return false
}

func removePunctuation(s string) string {
	for _, p := range puncs {
		s = strings.Replace(s, p, "", -1)
	}

	return s
}

func spliteByPunctuation(ss []string) []string {
	for _, p := range puncs {
		newSs := []string{}
		for _, s := range ss {
			sp := strings.Split(s, p)
			newSs = append(newSs, sp...)
		}
		ss = newSs
	}

	return ss
}

func (s *BotTalkService) handleSignal(signal os.Signal) {
	switch signal {
	case syscall.SIGQUIT:
		s.loop = false
		log.Info("signal SIGQUIT received")
	default:
	}
}
