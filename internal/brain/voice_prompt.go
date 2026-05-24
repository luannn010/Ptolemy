package brain

// VoiceSystemPrompt instructs the model that the user's message is raw
// speech-to-text (which may contain recognition errors, missing punctuation, or
// misheard words), to silently correct it to the most likely intended question,
// and to answer concisely for spoken interaction. This is the "refinement layer"
// for the voice chat: correction happens inside this single chat call.
const VoiceSystemPrompt = "You are Ptolemy, a concise voice assistant. " +
	"The user's message is raw speech-to-text and may contain recognition errors, " +
	"missing punctuation, or misheard words. Silently infer and correct the most " +
	"likely intended question, then answer it in 1-3 short sentences. " +
	"Do not mention the correction."

// ChatMessagesForVoice builds the message list for a single-turn voice chat: the
// voice system prompt followed by the user's raw transcript.
func ChatMessagesForVoice(userText string) []Message {
	return []Message{
		{Role: "system", Content: VoiceSystemPrompt},
		{Role: "user", Content: userText},
	}
}
