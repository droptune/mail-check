# mchk
[![ru](https://img.shields.io/badge/lang-ru-green.svg)](https://github.com/droptune/mail-check/blob/master/README.ru.md)

Simple program written in Go to test your email service.

Sends test message through the specified server and checks by IMAP if this message was received.

You can specify several tests and assert that message passes through (or not, if it is what you desire) while sent from different email providers.

It is created to test that everything works after changes to email server configuration.

## Usage

Put tests in `mchk.yml` configuration file (see next section) and run `mchk`.

## Configuration

Configuration is in [YAML](https://yaml.org/) format. On the first run example configuration file will be put into `~/.config/mchk/config.yml`.

Program searches for configuration in those locations in this order:
 - `~/.config/mchk/mchk.yml`
 - `~/.config/mchk.yml`
 - `mchk.yml` into current dir

Example configuration:

```yaml
---
# stop if errors encountered, or continue to next test
continue_on_errors: no
tests:
  - name: testing service 1
    # Set to 'no' if you want to test blocking of this specified sender
    should_send: yes
    smtp_server: smtp.example.com
    # Port is optional, default 25
    smtp_port: 25
    send_from: user@example.com
    send_to: user@example.com
    sender_login: user@example.com
    # If you skip this option, you will be prompted for password on each test run
    sender_password: password
    # Time to wait in seconds between sending message and checking if it is received
    wait_for: 2
    # Set to 'no' if you expect this message to be absent
    should_receive: yes
    imap_server: imap.example.com
    imap_tls: yes
    # imap_port is optional. Default is 993
    imap_port: 993
    imap_login: user@example.com
    # If you skip this option, you will be prompted for password on each test run
    imap_password: password
    # Deletes test message if set to 'no'
    leave_message: no
# other tests  if needed are configured in similar fashion
  - name: testing service 2
    # ...
```

You can omit passwords in config - then you will be prompted for them before tests are run.
