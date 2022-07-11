# sweeper

## Google Cloud Setup

- `gcloud projects create floorreport` - Create a new project
- `gcloud builds submit --tag gcr.io/floorreport/sweeper` - Build and submit to Google Container Registry
- `gcloud run deploy sweeper --image gcr.io/floorreport/sweeper --platform managed` - Deploy to Cloud Run

## Setup locally


- `gcloud iam service-accounts create local-dev` - Create service account
- `gcloud projects add-iam-policy-binding floorreport --member="serviceAccount:local-dev@floorreport.iam.gserviceaccount.com" --role="roles/owner"` - Create policy
- `gcloud iam service-accounts keys create credentials.json --iam-account=local-dev@floorreport.iam.gserviceaccount.com` - Create keys