import React, { useEffect, useRef, useState } from "react";
import "@azure/ai-vision-face-ui";

export function AzureLivenessCapture({ authToken, onComplete, onCancel, onError }) {
  const containerRef = useRef(null);
  const doneRef      = useRef(false);
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    if (!mounted || !containerRef.current) return;

    const el = document.createElement("azure-ai-vision-face-ui");
    el.style.width  = "100%";
    el.style.height = "100%";
    containerRef.current.appendChild(el);

    el.start(authToken)
      .then(() => {
        if (!doneRef.current) {
          doneRef.current = true;
          onComplete();
        }
      })
      .catch((err) => {
        if (!doneRef.current) {
          doneRef.current = true;
          const msg = err?.message ?? String(err);
          if (/cancel/i.test(msg)) {
            onCancel();
          } else {
            onError({ message: "Azure liveness error: " + msg });
          }
        }
      });

    return () => {
      if (containerRef.current) containerRef.current.innerHTML = "";
    };
  }, [mounted, authToken]);

  return (
    <div
      ref={containerRef}
      style={{ width: "100%", minHeight: "480px", background: "#000", borderRadius: "12px", overflow: "hidden" }}
    />
  );
}
