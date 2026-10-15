package streamkit

import (
	"strings"
	"testing"
)

func ttsMultilingualSegmentationSamples() []ttsSegmentationSample {
	plain := func(name, text string, protected ...string) ttsSegmentationSample {
		return ttsSegmentationSample{name: "multilingual/" + name, input: text, spoken: text, protected: protected}
	}
	samples := []ttsSegmentationSample{
		plain("traditional_chinese", "這筆金額是1,234.56元。明天再確認。", "1,234.56"),
		plain("code_switching", "请提醒Dr. Smith：明天10:15开会。Merci beaucoup！", "Dr. Smith", "10:15"),
		plain("japanese_quotes", "彼は「準備ができました。」と言った。次に進みます。", "できました。」"),
		plain("japanese_decimal", "合計は1,234.56円です。開始は10:15です。", "1,234.56", "10:15"),
		plain("korean_quotes", "그는 “준비가 끝났습니다.”라고 말했다. 이제 시작합니다.", "끝났습니다.”"),
		plain("korean_decimal", "가격은1,234.56원입니다. 회의는10:15에 시작합니다.", "1,234.56", "10:15"),
		plain("french_quotes", "Elle a dit : « Tout est prêt. » Nous pouvons partir.", "prêt. »"),
		plain("french_nonbreaking_quotes", "Elle a dit : «\u00a0Tout est prêt.\u00a0» Nous pouvons partir.", "prêt.\u00a0»"),
		plain("french_nested_quotes", "Elle a dit : « Il répond : « Tout est prêt. » » Puis elle part.", "prêt. » »"),
		plain("french_quotes_at_limit", strings.Repeat("é ", 36)+"« Tout est prêt. » Nous pouvons partir.", "prêt. »"),
		plain("french_decimal", "Le total est de 1\u202f234,56 euros. Merci beaucoup !", "1\u202f234,56"),
		plain("french_honorific", "Nous attendons M. Dupont aujourd'hui. Il arrive bientôt.", "M. Dupont", "aujourd'hui"),
		plain("german_quotes", "Er sagte: „Alles ist fertig.“ Danach gingen wir nach Hause.", "fertig.“"),
		plain("german_decimal", "Der Gesamtbetrag ist 1.234,56 Euro. Wir beginnen um 10:15 Uhr.", "1.234,56", "10:15"),
		plain("spanish_punctuation", "¿Está todo listo? ¡Sí, podemos empezar! Nos vemos mañana.", "¿Está", "¡Sí"),
		plain("spanish_decimal", "El total es de 1.234,56 euros. La reunión empieza a las 10:15.", "1.234,56", "10:15"),
		plain("italian_quotes", "Ha detto: «È tutto pronto.» Possiamo cominciare.", "pronto.»"),
		plain("italian_decimal", "Il totale è di 1.234,56 euro. L'incontro inizia alle 10:15.", "1.234,56", "L'incontro", "10:15"),
		plain("portuguese_quotes", "Ela disse: “Está tudo pronto.” Podemos começar agora.", "pronto.”"),
		plain("portuguese_decimal", "O total é de 1.234,56 euros. A reunião começa às 10:15.", "1.234,56", "10:15"),
		plain("dutch", "Alles is klaar. We kunnen nu beginnen! Tot morgen.", "kunnen"),
		plain("polish_decimal", "Łączna kwota to 1\u00a0234,56 zł. Spotkanie zaczyna się o 10:15.", "1\u00a0234,56", "10:15"),
		plain("turkish", "Her şey hazır mı? Evet, şimdi başlayabiliriz. Yarın görüşürüz.", "başlayabiliriz"),
		plain("vietnamese_combining", "To\u0302i đa\u0303 sa\u0306\u0303n sa\u0300ng. Chu\u0301ng ta ba\u0306\u0301t đa\u0302\u0300u!", "o\u0302", "a\u0306\u0303", "a\u0302\u0300"),
		plain("russian_quotes", "Он сказал: «Всё готово.» Мы можем начинать.", "готово.»"),
		plain("russian_decimal", "Итого 1\u00a0234,56 рубля. Встреча начнётся в 10:15.", "1\u00a0234,56", "10:15"),
		plain("russian_initials", "Автор А. С. Пушкин известен. Прочитаем его стихи.", "А. С. Пушкин"),
		plain("ukrainian", "Усе готово. Починаємо о 10:15! До зустрічі завтра.", "10:15", "зустрічі"),
		plain("greek", "Όλα είναι έτοιμα! Μπορούμε να αρχίσουμε. Τα λέμε αύριο.", "Μπορούμε"),
		plain("greek_initial", "Μιλάμε με τον Ν. Παπαδόπουλο σήμερα. Θα έρθει σύντομα.", "Ν. Παπαδόπουλο"),
		plain("arabic_punctuation", "قال: «كل شيء جاهز؟» نعم، يمكننا البدء. نلتقي غدًا.", "جاهز؟»", "غدًا"),
		plain("arabic_decimal", "المجموع هو ١٬٢٣٤٫٥٦. يبدأ الاجتماع في ١٠:١٥.", "١٬٢٣٤٫٥٦", "١٠:١٥"),
		plain("persian", "همه آماده‌اند؟ بله، می‌توانیم شروع کنیم. فردا می‌بینمت.", "آماده‌اند", "می‌توانیم", "می‌بینمت"),
		plain("persian_decimal", "مبلغ کل ۱٬۲۳۴٫۵۶ است. جلسه ساعت ۱۰:۱۵ شروع می‌شود.", "۱٬۲۳۴٫۵۶", "۱۰:۱۵", "می‌شود"),
		plain("hebrew", "הכול מוכן? כן, אפשר להתחיל! נתראה מחר.", "להתחיל"),
		plain("hebrew_decimal", "הסכום הכולל הוא 1,234.56 שקלים. הפגישה מתחילה בשעה 10:15.", "1,234.56", "10:15"),
		plain("urdu", "کیا سب تیار ہیں؟ جی ہاں، ہم شروع کر سکتے ہیں۔ کل ملیں گے۔", "ہیں؟", "ہیں۔"),
		plain("hindi_danda", "सब कुछ तैयार है। अब हम शुरू कर सकते हैं। कल फिर मिलेंगे।", "तैयार", "मिलेंगे"),
		plain("hindi_digits", "कुल राशि १,२३४.५६ रुपये है। बैठक १०:१५ पर शुरू होगी।", "१,२३४.५६", "१०:१५"),
		plain("bengali", "সব প্রস্তুত। আমরা এখন শুরু করতে পারি। আগামীকাল দেখা হবে।", "প্রস্তুত"),
		plain("bengali_digits", "মোট পরিমাণ ১,২৩৪.৫৬ টাকা। সভা ১০:১৫ মিনিটে শুরু হবে।", "১,২৩৪.৫৬", "১০:১৫"),
		plain("marathi", "सर्व तयारी झाली आहे। आता आपण सुरुवात करू शकतो। उद्या भेटूया।", "तयारी", "सुरुवात"),
		plain("tamil", "எல்லாம் தயார். நாம் இப்போது தொடங்கலாம்! நாளை சந்திப்போம்.", "இப்போது", "சந்திப்போம்"),
		plain("thai", "ทุกอย่างพร้อมแล้ว! เราเริ่มได้เลย พบกันพรุ่งนี้.", "พรุ่งนี้"),
		plain("indonesian", "Semuanya sudah siap. Kita bisa mulai sekarang! Sampai besok.", "sekarang"),
		plain("swahili", "Kila kitu kiko tayari. Tunaweza kuanza sasa! Tuonane kesho.", "Tunaweza"),
	}
	for _, sample := range []struct {
		name, prefix, cluster string
	}{
		{"japanese_combining", strings.Repeat("あ", 79), "か\u3099"},
		{"korean_jamo", strings.Repeat("가", 79), "한"},
		{"hindi_conjunct", strings.Repeat("अ", 79), "क्\u200dष"},
		{"bengali_conjunct", strings.Repeat("অ", 79), "ক্ষ"},
		{"arabic_marks", strings.Repeat("ا", 79), "ن\u0651\u064e"},
		{"thai_mark", strings.Repeat("ก", 79), "ก้"},
		{"vietnamese_marks", strings.Repeat("a", 79), "a\u0306\u0303"},
		{"french_grouped_number", strings.Repeat("é ", 39), "1\u202f234,56"},
	} {
		samples = append(samples, plain(sample.name+"_at_limit", sample.prefix+sample.cluster+"。", sample.cluster))
	}
	return samples
}

func TestTTSSentenceSegmenterCommitsMultilingualSentence(t *testing.T) {
	for _, sample := range []struct{ name, phrase, next string }{
		{"traditional_chinese", "這件事情已經完成。", "明"},
		{"japanese", "彼は「準備ができました。」", "次"},
		{"korean", "모든 준비가 끝났습니다.", "다"},
		{"french", "Elle a dit : « Tout est prêt. »", "N"},
		{"french_nonbreaking_quotes", "Elle a dit : «\u00a0Tout est prêt.\u00a0»", "N"},
		{"german_quotes", "Er sagte: „Alles ist fertig.“", "D"},
		{"spanish", "¿Está todo listo?", "¡"},
		{"italian", "Ha detto: «È tutto pronto.»", "P"},
		{"portuguese", "Está tudo pronto.", "P"},
		{"dutch", "Alles is klaar.", "W"},
		{"polish", "Wszystko jest gotowe.", "M"},
		{"turkish", "Her şey hazır mı?", "E"},
		{"vietnamese", "To\u0302i đa\u0303 sa\u0306\u0303n sa\u0300ng.", "C"},
		{"russian", "Он сказал: «Всё готово.»", "М"},
		{"ukrainian", "Усе готово.", "П"},
		{"greek", "Όλα είναι έτοιμα!", "Μ"},
		{"arabic", "كل شيء جاهز؟", "ن"},
		{"persian", "همه آماده‌اند؟", "ب"},
		{"hebrew", "הכול מוכן?", "כ"},
		{"urdu", "ہم شروع کر سکتے ہیں۔", "ک"},
		{"hindi", "सब कुछ तैयार है।", "अ"},
		{"bengali", "সব প্রস্তুত।", "আ"},
		{"marathi", "सर्व तयारी झाली आहे।", "आ"},
		{"tamil", "எல்லாம் தயார்.", "ந"},
		{"thai", "ทุกอย่างพร้อมแล้ว!", "เ"},
		{"indonesian", "Semuanya sudah siap.", "K"},
		{"swahili", "Kila kitu kiko tayari.", "T"},
	} {
		for _, lookahead := range []struct{ name, text string }{{"next_word", " " + sample.next}, {"paused", " "}} {
			t.Run(sample.name+"/"+lookahead.name, func(t *testing.T) {
				segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
				var emitted []string
				// Even a punctuation mark or closing quote may span transport chunks.
				for index := range len(sample.phrase) {
					segmenter.WriteString(sample.phrase[index : index+1])
					emitted = append(emitted, segmenter.Segments(false)...)
				}
				segmenter.WriteString(lookahead.text)
				got := append(emitted, segmenter.Segments(false)...)
				if len(got) == 0 || strings.TrimRight(strings.Join(got, ""), " ") != sample.phrase {
					t.Fatalf("complete sentence was not emitted before EOS: %q", got)
				}
				remaining := strings.Join(segmenter.Segments(true), "")
				want := sample.phrase
				if lookahead.text != " " {
					want += lookahead.text
				}
				if spoken := strings.Join(got, "") + remaining; spoken != want {
					t.Fatalf("text changed across sentence boundary: got %q, want %q", spoken, want)
				}
				if got := segmenter.Segments(true); len(got) != 0 {
					t.Fatalf("repeated flush duplicated text: %q", got)
				}
			})
		}
	}
}

func TestTTSSentenceSegmenterStreamsMultipleSentencesInsideFrenchQuote(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
	prefix := "Elle dit : « La première phrase est finie. "
	segmenter.WriteString(prefix)
	emitted := segmenter.Segments(false)
	segmenter.WriteString("La")
	emitted = append(emitted, segmenter.Segments(false)...)
	if got := strings.TrimRight(strings.Join(emitted, ""), " "); got != strings.TrimRight(prefix, " ") {
		t.Fatalf("first sentence waited for the closing quote: %q", emitted)
	}
	tail := " deuxième phrase suit. »"
	segmenter.WriteString(tail)
	ttsCheckSegments(t, append(emitted, segmenter.Segments(true)...), prefix+"La"+tail, []string{"suit. »"})
}

func TestTTSSentenceSegmenterRetainsMultilingualInitialBeforeName(t *testing.T) {
	for _, sample := range []struct{ name, prefix, suffix string }{
		{"french", "Nous attendons M. ", "Dupont aujourd'hui. "},
		{"russian", "Автор А. ", "С. Пушкин известен. "},
		{"greek", "Μιλάμε με τον Ν. ", "Παπαδόπουλο σήμερα. "},
	} {
		t.Run(sample.name, func(t *testing.T) {
			segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
			segmenter.WriteString(sample.prefix)
			if got := segmenter.Segments(false); len(got) != 0 {
				t.Fatalf("initial was emitted without its name: %q", got)
			}
			segmenter.WriteString(sample.suffix)
			want := strings.TrimRight(sample.prefix+sample.suffix, " ")
			if got := strings.Join(segmenter.Segments(false), ""); got != want {
				t.Fatalf("completed sentence was not emitted: got %q, want %q", got, want)
			}
		})
	}
}

func TestTTSSentenceSegmenterResetsFrenchQuoteAtMessageEnd(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
	unfinished := "« " + strings.Repeat("mot ", 24)
	segmenter.WriteString(unfinished)
	emitted := segmenter.Segments(false)
	if len(emitted) == 0 {
		t.Fatal("long quoted text was not streamed")
	}
	ttsCheckSegments(t, append(emitted, segmenter.Segments(true)...), strings.TrimRight(unfinished, " "), nil)
	phrase := "La nouvelle réponse est prête."
	segmenter.WriteString(phrase + " ")
	if got := strings.Join(segmenter.Segments(false), ""); got != phrase {
		t.Fatalf("quote state crossed the message boundary: %q", got)
	}
}
