# Manual do Cliente (Agentes)

Como entregar e instalar os "espiões" / agentes nas máquinas do seu cliente final.

Lembre-se: O executável construído DEVE estar alterado no código-fonte em `app.go` (linha ~54) conectando o gRPC para o endereço IP_DA_VPS da sua máquina cloud! Não distribua a versão compilada que contenha apontamento para "localhost".

---

## Para Ambientes Windows (Telinha Visual para o Cliente Instalar)

O "auto-monitor-client-desktop" (Wails) é perfeito para estações Windows (terminais, caixas) onde os funcionários e franqueados operam.

### Como Gerar o ".exe" no seu Linux (Cross-Compilation):
O Wails v2 permite que você, no Linux (PopOS/Ubuntu), crie o arquivo instalador final de Windows:

1. Será necessário instalar a ferramenta de build C do Windows no seu Linux. No seu terminal, rode:
   ```bash
   sudo apt install mingw-w64
   ```
2. Após instalar, estando na pasta `client-desktop/`:
   ```bash
   wails build -platform windows/amd64
   ```
3. Pegue o seu instalador único `.exe` gerado dentro da pastinha `build/bin/`. Você pode subir isso no Google Drive ou enviar pra eles instalarem como um programa simples via duplo-clique.

*(Opcional - Instalador "Próximo > Instalar" Completo):*
Se você quiser aquele instalador "Setup_X.msi/exe" que vai pro iniciar:
```bash
wails build -platform windows/amd64 -nsis
```

### Como o seu cliente utiliza:
Basta ele dar dois cliques no programa que você mandou, inserir o **CÓDIGO APP** que você cadastrou no seu painel SaaS, e pronto! O agente visualiza CPU e informa se está ok.

---

## Para Ambientes Linux e Servidores Sem-Tela (Headless e VPS do Cliente)

A Telinha do Wails exige interface visual e ambiente gráfico para "abrir". Se você tem um servidor Linux para monitorar (Ex: O BD principal de um cliente na nuvem), use o "Agent CLI/Headless".

### Como gerar o Binário Linux:

1. Neste caso vamos compilar o cliente CLI original. Abra seu bash e vá para pasta \`auto-monitor/client\`:
   ```bash
   cd auto-monitor/client
   ```
2. Antes compile apontando para sua api (aqui também é imperativo trocar de *localhost* para *IP_DA_VPS* no arquivo `main.go`). E então rode o build:
   ```bash
   go build -o auto-monitor-agent .
   ```
3. Mande este binário `auto-monitor-agent` gerado para a VPS remota a ser monitorada.

### Como "Instalar" (via systemd para subir sempre junto do boot):

1. Mande o arquivo compilado para a VPS do cliente em `/usr/local/bin/auto-monitor-agent`.
2. Dê permissão: `sudo chmod +x /usr/local/bin/auto-monitor-agent`
3. Crie um arquivo para rodar em *background*:
   `sudo nano /etc/systemd/system/automain-agent.service`
4. Coloque a seguinte definição no arquivo (onde a ENV `CODE_APP` conterá o código da máquina cadastrado por você):
   ```ini
   [Unit]
   Description=Auto Monitor Agent (Headless)
   After=network.target

   [Service]
   Type=simple
   Restart=always
   RestartSec=5
   Environment="CODE_APP=sua_licenca_aqui"
   ExecStart=/usr/local/bin/auto-monitor-agent -headless

   [Install]
   WantedBy=multi-user.target
   ```
5. Por fim inicie a vigia ativando a rotina autônoma:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable automain-agent
   sudo systemctl start automain-agent
   ```
6. Você não ouvirá falar na máquina, mas o daemon estará caladinho enviando telemetria p/ Vercel sem o usuário ver ou saber, garantindo a licença.
