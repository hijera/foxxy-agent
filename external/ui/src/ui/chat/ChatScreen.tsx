import { useEffect, useRef } from 'react';
import type { TokenUsage, TranscriptItem } from './types';
import { ChatHeader } from './ChatHeader';
import { Composer } from './Composer';
import { TokenBar } from './TokenBar';
import { MessageList } from '../messages/MessageList';

export function ChatScreen(props: {
  title: string;
  sessionId: string;
  onTitleSave: (title: string) => void;
  items: TranscriptItem[];
  draft: string;
  tokenUsage: TokenUsage | null;
  mode: string;
  modes: string[];
  onModeChange: (mode: string) => void;
  onDraftChange: (v: string) => void;
  onSend: (text: string) => void;
}) {
  const messagesRef = useRef<HTMLDivElement | null>(null);
  const isEmpty = props.items.length === 0;

  useEffect(() => {
    if (messagesRef.current) {
      messagesRef.current.scrollTop = messagesRef.current.scrollHeight;
    }
  }, [props.items]);

  return (
    <main className={`main ${isEmpty ? 'is-empty' : ''}`}>
      {isEmpty ? null : <ChatHeader title={props.title} editable={true} onTitleSave={props.onTitleSave} />}

      {isEmpty ? (
        <div className="hero" id="hero">
          <h1 className="hero-title">What do you want to know?</h1>
          <div className="hero-composer">
            <Composer
              value={props.draft}
              isEmpty={true}
              mode={props.mode}
              modes={props.modes}
              onModeChange={props.onModeChange}
              onChange={props.onDraftChange}
              onSend={props.onSend}
            />
          </div>
        </div>
      ) : (
        <div className="chat-stack">
          <div id="messages" className="messages" aria-live="polite" ref={messagesRef}>
            <div className="messages-inner">
              <MessageList items={props.items} />
            </div>
          </div>

          <div className="chat-bottom">
            <div className="chat-bottom-inner">
              <TokenBar usage={props.tokenUsage} />
              <Composer
                value={props.draft}
                isEmpty={false}
                mode={props.mode}
                modes={props.modes}
                onModeChange={props.onModeChange}
                onChange={props.onDraftChange}
                onSend={props.onSend}
              />
            </div>
          </div>
        </div>
      )}
    </main>
  );
}
