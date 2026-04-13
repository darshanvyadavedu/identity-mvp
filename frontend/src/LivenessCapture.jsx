import React, { useEffect, useRef } from "react";
import { FaceLivenessDetector } from "@aws-amplify/ui-react-liveness";
import "@aws-amplify/ui-react/styles.css";

const REGION = import.meta.env.VITE_AWS_REGION || "us-east-1";
// If still "Connecting..." after this many ms, force a timeout error.
const CONNECTION_TIMEOUT_MS = 20_000;

async function credentialProvider() {
  return {
    accessKeyId: import.meta.env.VITE_AWS_ACCESS_KEY_ID,
    secretAccessKey: import.meta.env.VITE_AWS_SECRET_ACCESS_KEY,
  };
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
    } else {
      onError(err);
    }
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
