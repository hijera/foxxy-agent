import { Fragment, useSyncExternalStore, type ReactNode } from "react";
import { snapshotEnv, subscribeEnv, switchGeneration } from "./remoteEnv";
import { serverEventsScope } from "../chat/sharedServerEvents";

/**
 * EnvScope starts the app over when the server it talks to, or the credential
 * it talks with, changes in place: a switch between two remotes, the same one
 * chosen again (remoteEnv.switchTo), or a token rotated under the one in use
 * (configuredRemotes.syncActiveToken). Everything the app read belongs to the
 * server it read it from, and the events stream and every other long-lived
 * reader hold the token they started with, so they start again; the page - and
 * what is on screen until the new answers come - stays.
 *
 * The key is the events stream's own scope (the server and a fingerprint of the
 * token, never the token) and the count of switches made in place.
 */
export function EnvScope(props: { children: ReactNode }) {
  const env = useSyncExternalStore(subscribeEnv, snapshotEnv, snapshotEnv);
  const key = `${serverEventsScope(env)}#${switchGeneration()}`;
  return <Fragment key={key}>{props.children}</Fragment>;
}
