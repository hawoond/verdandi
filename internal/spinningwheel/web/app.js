const canvas = document.getElementById("map");
const ctx = canvas.getContext("2d");
const runSelect = document.getElementById("runSelect");
const replayButton = document.getElementById("replayButton");
const timeline = document.getElementById("timeline");
const inspector = document.getElementById("inspector");

const zones = {
  planner: { x: 190, y: 165, label: "Planning desk", color: "#f2d17b" },
  "code-writer": { x: 555, y: 165, label: "Coding table", color: "#88c7f0" },
  tester: { x: 910, y: 165, label: "Testing lab", color: "#d99ddb" },
  documenter: { x: 360, y: 510, label: "Docs library", color: "#b89af0" },
  deployer: { x: 760, y: 510, label: "Deploy gate", color: "#f09b72" },
};

const animalGlyphs = {
  cat: "🐱",
  dog: "🐶",
  rabbit: "🐰",
  fox: "🦊",
  penguin: "🐧",
};

const agents = new Map();
let events = [];
let replayTimer = null;

function resizeCanvas() {
  const rect = canvas.getBoundingClientRect();
  const ratio = window.devicePixelRatio || 1;
  canvas.width = Math.max(900, Math.floor(rect.width * ratio));
  canvas.height = Math.max(540, Math.floor(rect.height * ratio));
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
  draw();
}

function scalePoint(point) {
  const rect = canvas.getBoundingClientRect();
  return {
    x: (point.x / 1100) * rect.width,
    y: (point.y / 720) * rect.height,
  };
}

function draw() {
  const rect = canvas.getBoundingClientRect();
  ctx.clearRect(0, 0, rect.width, rect.height);
  drawGround(rect);
  Object.entries(zones).forEach(([stage, zone]) => drawZone(stage, zone));
  agents.forEach(drawAgent);
}

function drawGround(rect) {
  ctx.fillStyle = "#a8d7b1";
  ctx.fillRect(0, 0, rect.width, rect.height);
  ctx.strokeStyle = "rgba(42, 74, 52, 0.18)";
  ctx.lineWidth = 1;
  for (let x = 0; x < rect.width; x += 36) {
    ctx.beginPath();
    ctx.moveTo(x, 0);
    ctx.lineTo(x, rect.height);
    ctx.stroke();
  }
  for (let y = 0; y < rect.height; y += 36) {
    ctx.beginPath();
    ctx.moveTo(0, y);
    ctx.lineTo(rect.width, y);
    ctx.stroke();
  }
}

function drawZone(stage, zone) {
  const point = scalePoint(zone);
  ctx.fillStyle = zone.color;
  roundRect(point.x - 92, point.y - 52, 184, 104, 8);
  ctx.fill();
  ctx.strokeStyle = "rgba(23, 32, 26, 0.28)";
  ctx.stroke();
  ctx.fillStyle = "#17201a";
  ctx.font = "700 14px system-ui";
  ctx.textAlign = "center";
  ctx.fillText(zone.label, point.x, point.y + 4);
  ctx.font = "12px system-ui";
  ctx.fillText(stage, point.x, point.y + 24);
}

function drawAgent(agent) {
  const point = scalePoint(agent.position);
  ctx.fillStyle = "rgba(0, 0, 0, 0.18)";
  ctx.beginPath();
  ctx.ellipse(point.x, point.y + 32, 32, 10, 0, 0, Math.PI * 2);
  ctx.fill();
  ctx.fillStyle = "#fffaf0";
  ctx.beginPath();
  ctx.arc(point.x, point.y, 32, 0, Math.PI * 2);
  ctx.fill();
  ctx.strokeStyle = agent.status === "error" ? "#c24137" : "#2e6f50";
  ctx.lineWidth = 4;
  ctx.stroke();
  ctx.font = "32px serif";
  ctx.textAlign = "center";
  ctx.fillText(animalGlyphs[agent.avatar] || "🐾", point.x, point.y + 11);
  ctx.fillStyle = "#17201a";
  ctx.font = "700 12px system-ui";
  ctx.fillText(agent.name, point.x, point.y + 54);
  if (agent.message) {
    ctx.fillStyle = "#fffaf0";
    roundRect(point.x - 76, point.y - 76, 152, 34, 8);
    ctx.fill();
    ctx.strokeStyle = "rgba(23, 32, 26, 0.18)";
    ctx.stroke();
    ctx.fillStyle = "#17201a";
    ctx.font = "12px system-ui";
    ctx.fillText(agent.message.slice(0, 22), point.x, point.y - 54);
  }
}

function roundRect(x, y, width, height, radius) {
  ctx.beginPath();
  ctx.moveTo(x + radius, y);
  ctx.arcTo(x + width, y, x + width, y + height, radius);
  ctx.arcTo(x + width, y + height, x, y + height, radius);
  ctx.arcTo(x, y + height, x, y, radius);
  ctx.arcTo(x, y, x + width, y, radius);
  ctx.closePath();
}

async function loadRuns() {
  const response = await fetch("/api/runs");
  const payload = await response.json();
  runSelect.innerHTML = "";
  payload.runs.forEach((run) => {
    const option = document.createElement("option");
    option.value = run.runId;
    option.textContent = `${run.status} · ${run.request || run.runId}`;
    runSelect.appendChild(option);
  });
  if (payload.runs.length > 0) {
    await loadEvents(payload.runs[0].runId);
  }
}

async function loadEvents(runId) {
  const response = await fetch(`/api/runs/${runId}/events`);
  const payload = await response.json();
  events = payload.events || [];
  agents.clear();
  timeline.innerHTML = "";
  draw();
}

function replay() {
  clearInterval(replayTimer);
  agents.clear();
  timeline.innerHTML = "";
  let index = 0;
  replayTimer = setInterval(() => {
    if (index >= events.length) {
      clearInterval(replayTimer);
      return;
    }
    applyEvent(events[index]);
    index += 1;
  }, 650);
}

function applyEvent(event) {
  if (event.agent) {
    const current = agents.get(event.agent.name) || {
      name: event.agent.name,
      avatar: event.agent.avatar.kind,
      position: { x: 90 + agents.size * 54, y: 90 },
      status: "active",
      message: "",
    };
    const zone = zones[event.stage];
    if (zone) {
      current.position = {
        x: zone.x + ((agents.size % 3) - 1) * 42,
        y: zone.y + 8 + Math.floor(agents.size / 3) * 36,
      };
    }
    current.status = event.status || current.status;
    current.message = event.message || event.type;
    current.role = event.agent.role;
    current.metrics = event.metrics;
    current.decision = event.decision;
    agents.set(event.agent.name, current);
    renderInspector(current);
  }
  appendTimeline(event);
  draw();
}

function appendTimeline(event) {
  const item = document.createElement("li");
  const label = event.agent ? event.agent.name : event.runId;
  item.innerHTML = `<strong>${event.type}</strong><span>${label}</span><p>${event.message || event.stage || ""}</p>`;
  timeline.appendChild(item);
  timeline.scrollLeft = timeline.scrollWidth;
}

function renderInspector(agent) {
  inspector.innerHTML = `
    <h2>${agent.name}</h2>
    <p>${animalGlyphs[agent.avatar] || "🐾"} ${agent.role || "agent"}</p>
    <p>Status: ${agent.status}</p>
    <p>${agent.message || ""}</p>
    ${agent.decision ? `<p>Decision: ${agent.decision.action} (${agent.decision.source || "unknown"})</p>` : ""}
    ${agent.metrics ? `<p>Success rate: ${Math.round((agent.metrics.successRate || 0) * 100)}%</p>` : ""}
  `;
}

runSelect.addEventListener("change", () => loadEvents(runSelect.value));
replayButton.addEventListener("click", replay);
window.addEventListener("resize", resizeCanvas);

resizeCanvas();
loadRuns().then(replay).catch((error) => {
  inspector.innerHTML = `<h2>Spinning Wheel</h2><p>${error.message}</p>`;
});
