const express = require('express');
const axios = require('axios');
const app = express();
const port = 8080;

const services = [
  {
    name: 'Nginx Orion (1026)',
    proxy: 'http://nginx-orion:1026/health',
    backends: [
      'http://orion-1:1027/version',
      'http://orion-2:1028/version',
      'http://orion-3:1029/version'
    ]
  },
  {
    name: 'Nginx IoT Agent (7896)',
    proxy: 'http://nginx-iot-7896:7896/health',
    backends: [
      'http://iotagent-1:7897/iot/about',
      'http://iotagent-2:7898/iot/about',
      'http://iotagent-3:7899/iot/about'
    ]
  },
  {
    name: 'Nginx IoT Agent (4041)',
    proxy: 'http://nginx-iot-4041:4041/health',
    backends: [
      'http://iotagent-1:4042/iot/about',
      'http://iotagent-2:4043/iot/about',
      'http://iotagent-3:4044/iot/about'
    ]
  }
];

async function checkHealth(url) {
  try {
    const response = await axios.get(url, { timeout: 2000 });
    return response.status === 200 ? 'UP' : 'DOWN';
  } catch (error) {
    return 'DOWN';
  }
}

app.get('/', async (req, res) => {
  let html = `
    <!DOCTYPE html>
    <html lang="en">
    <head>
      <meta charset="UTF-8">
      <meta name="viewport" content="width=device-width, initial-scale=1.0">
      <title>Service Status</title>
      <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        table { border-collapse: collapse; width: 100%; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        .up { color: green; }
        .down { color: red; }
      </style>
    </head>
    <body>
      <h1>Service Status</h1>
      <p>Timestamp: ${new Date().toLocaleString('en-US', { timeZone: 'Europe/Madrid' })}</p>
      <table>
        <tr>
          <th>Service</th>
          <th>Proxy Status</th>
          <th>Backend Status</th>
        </tr>
  `;

  for (const service of services) {
    const proxyStatus = await checkHealth(service.proxy);
    const backendStatuses = await Promise.all(service.backends.map(checkHealth));
    html += `
      <tr>
        <td>${service.name}</td>
        <td class="${proxyStatus.toLowerCase()}">${proxyStatus}</td>
        <td>
    `;
    service.backends.forEach((backend, index) => {
      html += `${backend}: <span class="${backendStatuses[index].toLowerCase()}">${backendStatuses[index]}</span><br>`;
    });
    html += `
        </td>
      </tr>
    `;
  }

  html += `
      </table>
    </body>
    </html>
  `;
  res.send(html);
});

app.listen(port, () => {
  console.log(`Status page running at http://localhost:${port}`);
});

