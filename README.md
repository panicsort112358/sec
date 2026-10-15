# sec

Read a filing as prose:

```sh
sec -cik 320193 -mode document -form 10-K
```

Choose a filing by accession number:

```sh
sec -cik 320193 -mode document -accession 0000320193-24-000123
```

Read extracted XBRL facts as JSON:

```sh
sec -cik 320193 -mode facts -form 10-K
```

Set `SEC_USER_AGENT` to a descriptive agent string with a contact email, or
pass `-ua 'Research App contact@example.com'`.
