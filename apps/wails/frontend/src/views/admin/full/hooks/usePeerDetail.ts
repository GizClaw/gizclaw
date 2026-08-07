import { useCallback, useEffect, useState } from "react";

import { expectData, toMessage } from "@/dashboard";
import {
  getPeer,
  getPeerInfo,
  getPeerRuntime,
  type DeviceInfo,
  type PeerRegistrationResult,
  type Runtime,
} from "@gizclaw/gizclaw/admin";

export interface PeerDetail {
  info: DeviceInfo | null;
  registration: PeerRegistrationResult | null;
  runtime: Runtime | null;
}

export interface PeerDetailState {
  data: PeerDetail | null;
  error: string;
  loading: boolean;
}

export function usePeerDetail(
  publicKey: string | undefined,
): PeerDetailState & { reload: () => Promise<void> } {
  const [state, setState] = useState<PeerDetailState>({
    data: null,
    error: "",
    loading: false,
  });

  const load = useCallback(async () => {
    if (publicKey === undefined || publicKey === "") {
      setState({ data: null, error: "", loading: false });
      return;
    }

    setState({ data: null, error: "", loading: true });
    try {
      const registration = await expectData(getPeer({ path: { publicKey } }));
      if (registration.status === "deleted") {
        setState({
          data: { info: null, registration, runtime: null },
          error: "",
          loading: false,
        });
        return;
      }
      const [info, runtime] = await Promise.all([
        loadOptional(() => expectData(getPeerInfo({ path: { publicKey } }))),
        loadOptional(() => expectData(getPeerRuntime({ path: { publicKey } }))),
      ]);

      setState({
        data: {
          info,
          registration,
          runtime,
        },
        error: "",
        loading: false,
      });
    } catch (error) {
      setState({
        data: null,
        error: toMessage(error),
        loading: false,
      });
    }
  }, [publicKey]);

  useEffect(() => {
    void load();
  }, [load]);

  return { ...state, reload: load };
}

async function loadOptional<T>(load: () => Promise<T>): Promise<T | null> {
  try {
    return await load();
  } catch {
    return null;
  }
}
