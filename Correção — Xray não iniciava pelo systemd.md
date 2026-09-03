# Correção — Xray não iniciava pelo systemd

## Problema

O serviço `xray.service` estava falhando ao iniciar pelo systemd com:

```text
Main process exited, code=exited, status=23
```

O Xray funcionava normalmente quando executado manualmente como `root`, mas falhava quando iniciado pelo systemd.

## Diagnóstico

O serviço estava configurado para executar com o usuário:

```ini
User=nobody
```

Ao testar manualmente com o mesmo usuário:

```bash
sudo -u nobody /usr/local/bin/xray run -config /usr/local/etc/xray/config.json
```

foi identificado o erro real:

```text
failed to initialize access logger
open /var/log/v2ray/access.log: permission denied
```

O usuário `nobody` não possuía permissão para escrever no diretório de logs.

## Correção

Foi alterada a propriedade do diretório de logs para o usuário utilizado pelo Xray:

```bash
chown -R nobody:nogroup /var/log/v2ray
```

Após isso, o serviço voltou a iniciar normalmente:

```bash
systemctl restart xray
systemctl status xray
```

## Causa

O problema não estava no `ExecStart`, na configuração do systemd ou no arquivo `config.json`.

A causa era **permissão de escrita no arquivo `/var/log/v2ray/access.log`**, pois o Xray era executado como `nobody`.

## Resultado

✅ Xray iniciou normalmente pelo systemd  
✅ Exit code `23` eliminado  
✅ Logs de acesso funcionando  
✅ Execução mantida com o usuário não privilegiado `nobody`