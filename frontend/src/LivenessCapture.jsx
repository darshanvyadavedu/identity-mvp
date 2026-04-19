import React, { useEffect, useRef } from "react";
import { FaceLivenessDetector } from "@aws-amplify/ui-react-liveness";
import "@aws-amplify/ui-react/styles.css";

const REGION = import.meta.env.VITE_AWS_REGION || "us-east-1";
// If still "Connecting..." after this many ms, force a timeout error.
const CONNECTION_TIMEOUT_MS = 20_000;

async function credentialProvider() {
  const accessKeyId     = import.meta.env.VITE_AWS_ACCESS_KEY_ID;
  const secretAccessKey = import.meta.env.VITE_AWS_SECRET_ACCESS_KEY;
  const sessionToken    = import.meta.env.VITE_AWS_SESSION_TOKEN; // optional for temp creds
  if (!accessKeyId || !secretAccessKey) {
    throw new Error(
      "AWS credentials not configured. Set VITE_AWS_ACCESS_KEY_ID and VITE_AWS_SECRET_ACCESS_KEY in frontend/.env"
    );
  }
  return sessionToken
    ? { accessKeyId, secretAccessKey, sessionToken }
    : { accessKeyId, secretAccessKey };
}

export function LivenessCapture({ providerSessionId, onComplete, onCancel, onError }) {
  const timerRef     = useRef(null);
  const completedRef = useRef(false);

  useEffect(() => {
    // Start a watchdog — if the SDK hasn't called onAnalysisComplete or
    // onError within CONNECTION_TIMEOUT_MS, treat it as a connection failure.
    timerRef.current = setTimeout(() => {
      if (!completedRef.current) {
        onError({
          state: "CONNECTION_TIMEOUT",
          message: "Could not connect to AWS Rekognition. Check your network and try again.",
        });
      }
    }, CONNECTION_TIMEOUT_MS);

    return () => clearTimeout(timerRef.current);
  }, []);

  function handleComplete() {
    completedRef.current = true;
    clearTimeout(timerRef.current);
    onComplete();
  }

  function handleError(err) {
    completedRef.current = true;
    clearTimeout(timerRef.current);
    if (err?.state === "USER_CANCELLED") {
      onCancel();
      return;
    }
    // Flatten AWS error shape { state, error: { Message } } into a plain Error.
    const rawMsg = err?.error?.Message || err?.error?.message || err?.message;
    const state  = err?.state ?? "UNKNOWN";
    const msg    = rawMsg
      ? `${state}: ${rawMsg}`
      : `Liveness check failed (${state}). Check your AWS credentials and region config.`;
    onError(new Error(msg));
  }

  return (
    <FaceLivenessDetector
      sessionId={providerSessionId}
      region={REGION}
      onAnalysisComplete={handleComplete}
      onError={handleError}
      config={{ credentialProvider }}
    />
  );
}
