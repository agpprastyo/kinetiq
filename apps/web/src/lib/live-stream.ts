import { API_BASE_URL } from "./api";

export interface LiveEvent {
  event_id: string;
  name: string;
  project_id: string;
  timestamp: string;
  user_id?: string;
  distinct_id?: string;
  properties?: Record<string, unknown>;
}

export function connectLiveStream(
  projectId: string,
  onEvent: (event: LiveEvent) => void
): () => void {
  const url = `${API_BASE_URL}/projects/${projectId}/live`;
  const es = new EventSource(url, { withCredentials: true });

  es.onmessage = (event: MessageEvent) => {
    try {
      const parsed = JSON.parse(event.data);
      onEvent(parsed);
    } catch {
      // Ignore heartbeat/ping/comment lines
    }
  };

  return () => {
    es.close();
  };
}
