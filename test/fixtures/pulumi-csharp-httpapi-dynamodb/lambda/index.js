exports.handler = async (event) => {
  const payload = JSON.parse(event.body || "{}");
  return {
    statusCode: 202,
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ id: payload.id, status: "accepted" })
  };
};
