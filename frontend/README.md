# Frontend (Vite)

## Run

1) Start the Go backend on port 8080 (separate terminal):

```bash
AZURE_FACE_ENDPOINT="https://<your-face-endpoint>" \
AZURE_FACE_KEY="<<your-key>>" \
go run main.go
```

2) Start the frontend:

```bash
cd frontend
npm install
npm run dev
```

Open the URL printed by Vite (usually `http://localhost:5173`).

## Notes

- This project proxies `/api/*` to `http://localhost:8080`.
- Full liveness capture on Web requires the official Azure SDK `@azure/ai-vision-face-ui`, which is distributed via Microsoft’s registry and requires additional setup.
- If you get `403 UnsupportedFeature/LivenessDetection`, your Azure Face resource is not approved for liveness yet: apply at `https://aka.ms/facerecognition`.

