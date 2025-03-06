There are three basic challenge structure types:

1. **`static-attachment`**: This challenge type is static and provides the player **only** the `./dist` folder as a ZIP file.  
2. **`static-attachment-with-compose`**: This type provides the `./dist` folder as a ZIP file **and** grants the player access to a Docker Compose instance.  
3. **`static-container`**: This type provides the `./dist` folder as a ZIP file **and** deploys a single container instance per team.  

Choose the appropriate type based on your requirements.  

Each challenge should include four basic folders/files:  

- **`./dist`**:  
  - This folder is the **attachment provided to players**.  
  - **Remove flags** from this folder before distribution, as players will receive it.  
  - If no attachment is needed, delete the line `provide: "./dist"` from `challenge.yml`.  

- **`./solver`**:  
  - A folder for your challenge's solver script or a simplified explanation (TL;DR) of the solution.  

- **`./src`**:  
  - The source code for your challenge.  
  - If using Docker Compose, **randomize open ports** using a tool like the [CyberChef Random Port Generator](https://tool.cyberchef.dev/random-port-generator).  

- **`challenge.yml`**:  
  - The challenge configuration file. Update the following fields:  
    - **`name`**: Your challenge's name.  
    - **`author`**: Your Discord username.  
    - **`description`**: A brief description. If the challenge requires a connection, specify it here (e.g., `nc {{ .host }} 8888` or `http://{{ .host }}/`).  
    - **`flags`**: The flag(s) for the challenge. Multiple flag formats are allowed.  
