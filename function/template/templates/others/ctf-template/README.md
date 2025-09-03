# CTF Challenge Maker & Writeup Submission Guide

Format flag: `.*{.*}`
Maximum flag length: `64`

## Important Dates

- **CTF Start Date:** [CTF Start Date]
- **Challenge Submission Deadline:** [Challenge Submission Deadline]

## Instructions for Participants

If you successfully solve a challenge, please consider contributing a detailed writeup to help the community learn. When submitting your writeup, follow these guidelines:

- **Fork & Branch:** Fork the repository and create a new branch for your writeup submission.
- **Directory Structure:** Place your writeup in the appropriate challenge folder (e.g., `Web/<challenge_name>/writeups/<yourname>/`, `Crypto/<challenge_name>/writeups/<yourname>/`, etc.).
- **Content:** Ensure your writeup includes:
  - A brief overview of the challenge.
  - Steps of your approach, including any commands, code snippets, or screenshots.
- **Submission:** Open a Pull Request once your writeup is ready.

## Instructions for Challenge Authors

As a challenge author, it is highly recommended to include a solver to assist participants in verifying their solutions and understanding the intended approach. Please follow these guidelines:

- **Use Template Structure:** Base your challenge on the templates provided in the `.example/` directory:
  - `static-attachment/` - For challenges with static files/attachments
  - `static-attachment-with-compose/` - For challenges requiring Docker Compose setup
  - `static-attachment-with-compose-traefik/` - For challenges requiring custom domain routing, host SNI, or Traefik reverse proxy integration
  - `static-container/` - For challenges that need one container per person/team 
  - `dynamic-container/` - For challenges that need dynamic container per person/team

- **⚠️ Hints in challenge.yml:** You can add hints in your `challenge.yml` file if desired, but keep in mind that these hints will automatically become public on the platform once the challenge is published and will be visible to all participants.

### Automatic Challenge Updates

The CTF CI/CD supports **automatic challenge updates** when you push changes to the repository. Here's how it works:

#### **Update Types Based on File Location:**

- **📝 Configuration Changes** (`challenge.yml`): Updates challenge metadata, description, flags, and attachments
- **📁 Attachment Updates** (`dist/` folder): Updates only the challenge attachment files
- **🔄 Full Redeploy** (`src/` folder): Stops services, updates challenge, and restarts services
- **🐳 Infrastructure Changes** (`Dockerfile`, `docker-compose.yml`, `Makefile`): Complete redeploy with service restart
- **📚 Documentation** (`solver/`, `writeup/` folders): No updates (documentation only)

#### **How to Update Your Challenge:**

1. **Make your changes** to the appropriate files in your challenge directory
2. **Commit and push** your changes to the repository:
   ```bash
   git add .
   git commit -m "Update challenge: description and fix vulnerability"
   git push origin main
   ```
3. **Automatic deployment** happens within seconds based on what you changed:
   - Edit `challenge.yml` → Metadata update (~3 seconds)
   - Update `dist/files.zip` → Attachment update (~2 seconds)  
   - Modify `src/app.py` → Full redeploy (~10 seconds)