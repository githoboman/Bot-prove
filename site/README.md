# site/

Static one-pager for CasperProver. No build step — plain HTML + CSS.

## Local preview

```bash
python3 -m http.server -d site 8080
# open http://localhost:8080/
```

## Deploy

Any static host works: GitHub Pages, Vercel (static), Netlify, `nginx`.

The page intentionally has **no roadmap leak** — every claim on it is
either shipped or clearly labelled as SIMULATION.
