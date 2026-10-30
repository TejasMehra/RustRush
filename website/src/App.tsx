import { useState } from "react";

const repo = "https://github.com/TejasMehra/RustRush";
const docs = `${repo}/blob/main/`;
const base = import.meta.env.BASE_URL;

const sections = [
  { number: "01", label: "Method", href: "#method" },
  { number: "02", label: "Evidence", href: "#evidence" },
  { number: "03", label: "Start", href: "#try" },
];

const methodSteps = [
  {
    number: "01",
    title: "Choose one operation",
    body: "Use the guide to inspect a Go module. Its operation rankings come from source patterns, not production profiling; candidate runs also accept a single Python baseline file.",
  },
  {
    number: "02",
    title: "Write the Rust candidate",
    body: "Keep it small and deterministic. The candidate is a reviewed, dependency-free Rust file, not generated code.",
  },
  {
    number: "03",
    title: "Match the output",
    body: "Both warm-ups receive the same arguments and input. Different output stops the comparison before timed trials.",
  },
  {
    number: "04",
    title: "Read the measurements",
    body: "Paired runs alternate order. Raw process samples, percentiles, throughput, and quality warnings remain in the report.",
  },
];

function App() {
  const [menuOpen, setMenuOpen] = useState(false);

  const closeMenu = () => setMenuOpen(false);

  return (
    <>
      <a className="skip-link" href="#main">Skip to content</a>

      <header className="site-header">
        <div className="header-inner">
          <a className="brand" href="#top" aria-label="RustRush home" onClick={closeMenu}>
            <img src={`${base}rustrush-logo-transparent.png`} alt="" />
            <span>RustRush</span>
          </a>
          <span className="header-note">A tool for the rewrite decision</span>
          <button
            className="menu-toggle"
            type="button"
            aria-expanded={menuOpen}
            aria-controls="primary-navigation"
            onClick={() => setMenuOpen((open) => !open)}
          >
            <span>{menuOpen ? "Close" : "Menu"}</span>
            <span className="menu-glyph" aria-hidden="true">{menuOpen ? "−" : "+"}</span>
          </button>
          <nav
            className={`primary-nav${menuOpen ? " is-open" : ""}`}
            id="primary-navigation"
            aria-label="Main navigation"
          >
            <a href="#method" onClick={closeMenu}>Method</a>
            <a href="#evidence" onClick={closeMenu}>Reading a report</a>
            <a href="#try" onClick={closeMenu}>Try the demo</a>
            <a className="github-link" href={repo} target="_blank" rel="noreferrer" onClick={closeMenu}>
              GitHub <span aria-hidden="true">↗</span>
            </a>
          </nav>
        </div>
      </header>

      <main id="main">
        <div className="page-frame" id="top">
          <aside className="index-rail" aria-label="Page sections">
            <span className="rail-caption">FIELD<br />NOTES / 01</span>
            <nav aria-label="Section index">
              {sections.map((section) => (
                <a key={section.number} href={section.href}>
                  <span>{section.number}</span>
                  <span>{section.label}</span>
                </a>
              ))}
            </nav>
            <span className="rail-end">R/R</span>
          </aside>

          <div className="page-content">
            <section className="hero" aria-labelledby="hero-title">
              <div className="hero-copy">
                <p className="folio"><span>RUSHRUSH</span><span>BENCHMARK NOTES&nbsp; / &nbsp;001</span></p>
                <p className="hero-index">THE REWRITE QUESTION</p>
                <h1 id="hero-title">Before you rewrite it, <em>measure it.</em></h1>
                <p className="hero-lede">
                  Put a Go operation or single Python file beside a Rust candidate. RustRush checks that
                  their outputs match, then measures both before you commit to the work.
                </p>
                <div className="hero-actions">
                  <a className="action-link" href="#try">
                    <span>Run the local demo</span><span aria-hidden="true">↓</span>
                  </a>
                  <span className="action-note">Go 1.22+ · no API key</span>
                </div>
                <div className="hero-fact">
                  <span className="fact-rule" />
                  <p>A microbenchmark can inform one decision. It cannot predict an entire production system.</p>
                </div>
              </div>

              <figure className="comparison-sheet" aria-labelledby="sheet-title">
                <div className="sheet-top">
                  <span id="sheet-title">REPORT PREVIEW <span className="sheet-slash">/</span> SCHEMA 1.1</span>
                  <span className="sample-stamp">NO RUN ATTACHED</span>
                </div>
                <div className="sheet-title-row">
                  <div>
                    <p className="sheet-overline">ONE OPERATION · TWO IMPLEMENTATIONS</p>
                    <h2>Compare the work,<br />not the promise.</h2>
                  </div>
                  <div className="verdict">
                    <span>VERDICT</span>
                    <strong>INCONCLUSIVE</strong>
                  </div>
                </div>

                <div className="protocol-visual">
                  <div className="protocol-heading">
                    <span>COMPARISON SEQUENCE</span>
                    <span>ILLUSTRATION · NOT TO SCALE</span>
                  </div>
                  <svg viewBox="0 0 620 174" role="img" aria-labelledby="protocol-title protocol-desc">
                    <title id="protocol-title">Go or Python and Rust benchmark protocol</title>
                    <desc id="protocol-desc">
                      A schematic of both implementations receiving the same input,
                      passing through an output check, then entering timing. The
                      diagram contains no measured durations.
                    </desc>
                    <text className="svg-head" x="170" y="24">SAME INPUT</text>
                    <text className="svg-head" x="355" y="24">OUTPUT CHECK</text>
                    <text className="svg-head" x="520" y="24">TIMING</text>
                    <text className="svg-lane" x="4" y="72">BASELINE</text>
                    <text className="svg-sublane" x="4" y="88">baseline</text>
                    <text className="svg-lane" x="4" y="133">RUST</text>
                    <text className="svg-sublane" x="4" y="149">candidate</text>
                    <path className="svg-track" d="M92 76H557 M92 137H557" />
                    <path className="svg-shared" d="M170 76V137 M355 76V137 M520 76V137" />
                    <circle className="svg-node" cx="170" cy="76" r="5" />
                    <circle className="svg-node" cx="170" cy="137" r="5" />
                    <circle className="svg-gate" cx="355" cy="76" r="7" />
                    <circle className="svg-gate" cx="355" cy="137" r="7" />
                    <path className="svg-gate-link" d="M355 83V130" />
                    <path className="svg-clock" d="M512 76h16m-16 61h16" />
                    <path className="svg-tick" d="M543 65v22m14-22v22m-14 39v22m14-22v22" />
                    <text className="svg-foot" x="92" y="168">WARM-UP OUTPUTS MUST MATCH BEFORE MEASURED TRIALS</text>
                  </svg>
                </div>

                <div className="sheet-check">
                  <span className="check-mark" aria-hidden="true">!</span>
                  <span>Correctness gate</span>
                  <strong>NOT RUN</strong>
                </div>
                <table className="sheet-measurements" aria-label="Report measurement fields, unavailable because no benchmark is attached">
                  <thead>
                    <tr><th scope="col">PROCESS MEASUREMENT</th><th scope="col">BASELINE</th><th scope="col">RUST</th></tr>
                  </thead>
                  <tbody>
                    <tr><th scope="row">p50 / p95 / p99</th><td>n/a</td><td>n/a</td></tr>
                    <tr><th scope="row">Throughput</th><td>n/a</td><td>n/a</td></tr>
                    <tr><th scope="row">Timing mode</th><td>not recorded</td><td>not recorded</td></tr>
                  </tbody>
                </table>
                <figcaption className="sheet-caption">
                  <span>Illustrative interface.</span>
                  <span>No benchmark was run.</span>
                </figcaption>
              </figure>
            </section>

            <section className="method-section ruled-section" id="method" aria-labelledby="method-title">
              <div className="section-marker"><span>01</span><span>METHOD</span></div>
              <div className="method-layout">
                <div className="section-intro">
                  <p className="section-kicker">A small experiment, end to end</p>
                  <h2 id="method-title">Keep the question <em>narrow.</em></h2>
                  <p>RustRush works with a focused operation, not a whole service. You choose the candidate and the workload.</p>
                </div>
                <ol className="method-list">
                  {methodSteps.map((step) => (
                    <li key={step.number}>
                      <span className="step-number">{step.number}</span>
                      <div>
                        <h3>{step.title}</h3>
                        <p>{step.body}</p>
                      </div>
                      <span className="step-mark" aria-hidden="true">↗</span>
                    </li>
                  ))}
                </ol>
              </div>
            </section>

            <section className="evidence-section ruled-section" id="evidence" aria-labelledby="evidence-title">
              <div className="section-marker"><span>02</span><span>EVIDENCE</span></div>
              <div className="evidence-layout">
                <div className="section-intro">
                  <p className="section-kicker">A report is not a forecast</p>
                  <h2 id="evidence-title">Separate what ran from what was <em>assumed.</em></h2>
                  <p>JSON keeps the raw samples. Markdown explains the result. SVG gives you a compact summary.</p>
                  <a className="inline-link" href={`${docs}docs/report-schema.md`}>Read the report schema <span aria-hidden="true">↗</span></a>
                </div>
                <div className="evidence-ledger">
                  <div className="ledger-head">
                    <span>IN A RUSHRUSH REPORT</span>
                    <span>HOW TO READ IT</span>
                  </div>
                  <div className="ledger-row">
                    <span className="ledger-type measured">MEASURED</span>
                    <p>Process wall time (including startup in the default mode), throughput, raw samples, and runtime details. Percentiles are not per-request latency.</p>
                  </div>
                  <div className="ledger-row">
                    <span className="ledger-type modeled">MODELED</span>
                    <p>ROI arithmetic from your workload, CPU, cost, and effort assumptions. It is not a forecast.</p>
                  </div>
                  <div className="ledger-row inconclusive-row">
                    <span className="ledger-type inconclusive">INCONCLUSIVE</span>
                    <p>Missing, failed, mismatched, noisy, or low-resolution evidence.</p>
                  </div>
                  <p className="ledger-foot">No confidence intervals are computed. A matching checksum is not proof of full semantic equivalence.</p>
                </div>
              </div>
            </section>

            <aside className="principle" aria-label="RustRush's measurement principle">
              <span className="principle-index">A PRACTICAL LIMIT</span>
              <p>A benchmark can tell you what happened in one test. <em>It cannot tell you what your whole backend needs.</em></p>
            </aside>

            <section className="start-section ruled-section" id="try" aria-labelledby="start-title">
              <div className="section-marker"><span>03</span><span>START HERE</span></div>
              <div className="start-layout">
                <div className="section-intro">
                  <p className="section-kicker">Run it on your machine</p>
                  <h2 id="start-title">Start with the <em>demo.</em></h2>
                  <p>Go 1.22 or newer is enough for the demo. Candidate runs accept Go or a single Python baseline; a conclusive comparison also needs `rustc` on your PATH.</p>
                  <a className="inline-link" href={`${docs}docs/benchmark-methodology.md`}>Benchmark methodology <span aria-hidden="true">↗</span></a>
                </div>
                <div className="command-block">
                  <div className="command-heading">
                    <span>POWERSHELL / TERMINAL</span>
                    <span>NO API KEY</span>
                  </div>
                  <pre><code><span className="prompt">$</span> go run ./cmd/rustrush demo</code></pre>
                  <div className="command-output">
                    <span>Writes a current-run report to</span>
                    <code>results/latest.json</code>
                    <code>results/latest.md</code>
                    <code>results/latest.svg</code>
                  </div>
                  <p className="command-note">Without Rust, the baseline can still run. The report marks the missing candidate and returns <code>INCONCLUSIVE</code>.</p>
                </div>
              </div>
              <div className="container-note">
                <span className="container-symbol" aria-hidden="true">[]</span>
                <p><strong>Need container mode?</strong> It requires a working Docker environment and locally available images. Linux is verified; Docker Desktop on Windows and macOS is not.</p>
                <a href={`${docs}SECURITY.md`}>Security limits <span aria-hidden="true">↗</span></a>
              </div>
            </section>

            <footer className="site-footer">
              <a className="footer-brand" href="#top">
                <img src={`${base}rustrush-logo-transparent.png`} alt="" />
                <span>RustRush</span>
              </a>
              <p>A focused benchmark for one engineering decision.</p>
              <nav aria-label="Footer links">
                <a href={`${docs}README.md`}>Developer guide</a>
                <a href={`${docs}docs/roi-model.md`}>ROI model</a>
                <a href={repo} target="_blank" rel="noreferrer">GitHub <span aria-hidden="true">↗</span></a>
              </nav>
              <span className="footer-id">R/R · EXPERIMENTAL</span>
            </footer>
          </div>
        </div>
      </main>
    </>
  );
}

export default App;
