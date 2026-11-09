using Newtonsoft.Json;

namespace Fence.Protocol
{
    public static class ProtocolJson
    {
        // TypeNameHandling must stay None: anything else is a dynamic-code finding in the game's ModScanner.
        static readonly JsonSerializerSettings Settings = new JsonSerializerSettings
        {
            NullValueHandling = NullValueHandling.Ignore,
            MissingMemberHandling = MissingMemberHandling.Ignore,
            DateParseHandling = DateParseHandling.None,
            FloatParseHandling = FloatParseHandling.Double,
        };

        public static string Serialize(object value) => JsonConvert.SerializeObject(value, Formatting.None, Settings);

        public static T Deserialize<T>(string json) => JsonConvert.DeserializeObject<T>(json, Settings);
    }
}
