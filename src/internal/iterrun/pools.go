// Package iterrun: the plan-codename word pools.
//
// Six namespaces, each a letter -> words table, consumed in the order
// listed by namespaceOrder: a letter's words are drawn from animals
// first, then minerals, then cities, rivers, trees and stars. The pools
// are deliberately ragged -- a namespace carries as many REAL entries as
// that letter honestly has, never padded with invented words -- because
// depth is squared up at lookup time instead: PoolWord walks the
// concatenated list and, once it wraps, appends an occurrence number
// (quail, quokka, ..., quail2, quokka2, ...). That makes every letter
// effectively unbounded and identically deep, with no fabricated species
// in the table and no model in the path -- PoolWord is a slice index and
// a modulo.
//
// Nothing here is AI-assigned. NextPlanName claims from these tables
// under a file lock; ReleasePlanName hands one back.
package iterrun

import (
	"fmt"
	"io"
	"strings"
)

// namespaceOrder is the consumption order. Earlier namespaces are
// exhausted at a given letter before the next is touched, so a project's
// early plans are animals and the switch to minerals happens on its own
// as the animal column runs out.
var namespaceOrder = []string{"animals", "minerals", "cities", "rivers", "trees", "stars"}

// pools maps namespace -> letter -> words. Every word is a real member of
// that category; see the package comment for why the columns are uneven.
var pools = map[string]map[byte][]string{

	"animals": {
		'a': {"aardvark", "aardwolf", "addax", "adder", "agouti", "albatross", "alligator", "alpaca", "anaconda", "anchovy", "anemone", "angelfish", "anglerfish", "anhinga", "anoa", "anole", "ant", "anteater", "antelope", "ape", "aphid", "arapaima", "argali", "armadillo", "asp", "auk", "avocet", "axolotl"},
		'b': {"baboon", "badger", "bandicoot", "barnacle", "barracuda", "basilisk", "bass", "bat", "bear", "beaver", "bee", "beetle", "beluga", "bilby", "binturong", "bison", "bittern", "blackbird", "boa", "boar", "bobcat", "bongo", "bonobo", "booby", "bowerbird", "budgerigar", "buffalo", "bullfinch", "bunting", "bustard", "butterfly", "buzzard"},
		'c': {"caiman", "camel", "capybara", "caracal", "cardinal", "caribou", "carp", "cassowary", "caterpillar", "catfish", "centipede", "chameleon", "chamois", "cheetah", "chickadee", "chimpanzee", "chinchilla", "chipmunk", "chough", "cicada", "civet", "clam", "coati", "cobra", "cockatoo", "cod", "condor", "coot", "cormorant", "cougar", "coyote", "crab", "crane", "crayfish", "cricket", "crocodile", "crow", "cuckoo", "curlew", "cuttlefish"},
		'd': {"dace", "dachshund", "dalmatian", "damselfly", "darter", "deer", "degu", "desman", "dhole", "dikdik", "dingo", "dipper", "dodo", "dogfish", "dolphin", "donkey", "dorado", "dormouse", "dotterel", "dove", "dragonfly", "drill", "dromedary", "drongo", "duck", "dugong", "dunlin", "dunnart", "dunnock"},
		'e': {"eagle", "eagleray", "earthworm", "earwig", "echidna", "echinoderm", "eclectus", "eel", "eelpout", "eft", "egret", "eider", "eland", "elephant", "elephantseal", "elephantshrew", "elk", "elkhound", "elver", "emu", "ermine", "escolar", "eulachon", "ensatina", "eyra"},
		'f': {"falcon", "fantail", "fennec", "ferret", "fieldfare", "finch", "firefly", "fisher", "flamingo", "flatfish", "flea", "flicker", "flounder", "flycatcher", "flyingfish", "fossa", "fowl", "fox", "francolin", "frigatebird", "frillneck", "fritillary", "frog", "frogfish", "frogmouth", "fulmar", "furseal"},
		'g': {"gadwall", "galago", "gannet", "gar", "gaur", "gazelle", "gecko", "gelada", "gemsbok", "genet", "gerbil", "gerenuk", "gharial", "gibbon", "giraffe", "gnat", "gnu", "goat", "godwit", "goldcrest", "goldfinch", "goose", "gopher", "gorilla", "goshawk", "grackle", "grasshopper", "grebe", "grosbeak", "grouper", "grouse", "guanaco", "gull"},
		'h': {"haddock", "hake", "halibut", "hamster", "hare", "harrier", "hartebeest", "hawk", "hedgehog", "heron", "herring", "hoatzin", "hog", "hoopoe", "hornbill", "hornet", "horse", "hoverfly", "howler", "huchen", "humpback", "hummingbird", "husky", "hutia", "hyena", "hyrax"},
		'i': {"ibex", "ibis", "icefish", "ichneumon", "ide", "iguana", "iguanodon", "iiwi", "impala", "inchworm", "indigobird", "indri", "irukandji", "isopod", "ivorygull"},
		'j': {"jabiru", "jacamar", "jacana", "jackal", "jackdaw", "jackrabbit", "jaeger", "jaguar", "jaguarundi", "javelina", "jay", "jellyfish", "jerboa", "jewelfish", "jird", "junco", "junglefowl"},
		'k': {"kagu", "kakapo", "kangaroo", "katydid", "kea", "kestrel", "killdeer", "killifish", "kingbird", "kingfisher", "kinglet", "kinkajou", "kiskadee", "kite", "kittiwake", "kiwi", "klipspringer", "knot", "koala", "kob", "kodkod", "kookaburra", "kouprey", "krait", "krill", "kudu"},
		'l': {"lacewing", "ladybird", "lamprey", "langur", "lapwing", "lark", "leafhopper", "lechwe", "leech", "lemming", "lemur", "leopard", "limpet", "ling", "linnet", "lion", "lionfish", "lizard", "llama", "loach", "lobster", "locust", "loggerhead", "loon", "lorikeet", "loris", "lovebird", "lungfish", "lynx", "lyrebird"},
		'm': {"macaque", "macaw", "mackerel", "mallard", "mamba", "manatee", "mandrill", "manta", "mara", "margay", "marlin", "marmoset", "marmot", "marten", "mayfly", "meadowlark", "meerkat", "merganser", "merlin", "midge", "millipede", "mink", "minnow", "mockingbird", "mole", "mongoose", "monitor", "monkey", "moorhen", "moose", "moth", "mouflon", "mouse", "mudskipper", "mule", "mullet", "muntjac", "murre", "muskox", "muskrat", "mussel"},
		'n': {"nag", "narwhal", "natterjack", "nautilus", "needlefish", "nene", "newt", "nighthawk", "nightingale", "nightjar", "nilgai", "noctule", "noddy", "numbat", "nutcracker", "nuthatch", "nutria", "nyala"},
		'o': {"oarfish", "ocelot", "octopus", "oilbird", "okapi", "olingo", "olm", "onager", "opah", "opossum", "orangutan", "orca", "oribi", "oriole", "oryx", "osprey", "ostrich", "otter", "ouzel", "ovenbird", "owl", "ox", "oxpecker", "oystercatcher"},
		'p': {"paca", "paddlefish", "panda", "pangolin", "panther", "parakeet", "parrot", "parrotfish", "partridge", "peacock", "peccary", "pelican", "penguin", "perch", "peregrine", "petrel", "phalarope", "pheasant", "pig", "pika", "pike", "pintail", "pipefish", "pipit", "piranha", "pitta", "platypus", "plover", "porcupine", "porpoise", "possum", "potoo", "prawn", "ptarmigan", "puffin", "puma", "python"},
		'q': {"quagga", "quahog", "quail", "quailfinch", "quakerparrot", "quelea", "queensnake", "quetzal", "quillback", "quillfish", "quinnat", "quokka", "quoll"},
		'r': {"rabbit", "raccoon", "ragfish", "rail", "ram", "rasbora", "rat", "ratel", "rattlesnake", "raven", "ray", "razorbill", "redpoll", "redshank", "redstart", "redwing", "reedbuck", "reindeer", "remora", "rhea", "rhinoceros", "roach", "roadrunner", "robin", "rockfish", "roller", "rook", "rorqual", "rosella", "rotifer", "ruff"},
		's': {"sable", "saiga", "sailfish", "salamander", "salmon", "sanderling", "sandgrouse", "sandpiper", "sapsucker", "sardine", "sawfish", "scallop", "scaup", "scorpion", "scoter", "seahorse", "seal", "sealion", "serval", "shad", "shark", "shearwater", "sheep", "shelduck", "shrew", "shrike", "shrimp", "siamang", "sifaka", "siskin", "skate", "skimmer", "skink", "skua", "skunk", "sloth", "slug", "smelt", "snail", "snake", "snipe", "sole", "sparrow", "spoonbill", "springbok", "squid", "squirrel", "starling", "stingray", "stoat", "stork", "sturgeon", "sunbird", "swallow", "swan", "swift", "swordfish"},
		't': {"tahr", "takin", "tamandua", "tamarin", "tanager", "tapir", "tarantula", "tarpon", "tarsier", "teal", "tench", "tenrec", "termite", "tern", "terrapin", "tetra", "thrasher", "thrush", "tiger", "tilapia", "tinamou", "titmouse", "toad", "toadfish", "topi", "tortoise", "toucan", "towhee", "tragopan", "treecreeper", "trogon", "trout", "tuatara", "tuna", "turaco", "turkey", "turnstone", "turtle"},
		'u': {"uakari", "umbrellabird", "unau", "urchin", "urial", "uromastyx", "urubu", "urutu"},
		'v': {"vampirebat", "vanga", "vaquita", "veery", "velvetworm", "verdin", "vervet", "vicuna", "vinegaroon", "viper", "vireo", "viscacha", "vixen", "vole", "vulture"},
		'w': {"wagtail", "wallaby", "wallaroo", "walrus", "warbler", "warthog", "waterbuck", "wattlebird", "waxbill", "waxwing", "weasel", "weevil", "whale", "whimbrel", "whinchat", "whippet", "whiteeye", "whiting", "widgeon", "wildebeest", "willet", "wolf", "wolverine", "wombat", "woodchuck", "woodcock", "woodlouse", "woodpecker", "worm", "wrasse", "wren", "wryneck"},
		'x': {"xantus", "xenarthra", "xenopus", "xenops", "xerus", "xiphias", "xoloitzcuintli"},
		'y': {"yabby", "yaffle", "yak", "yapok", "yearling", "yellowhammer", "yellowjacket", "yellowlegs", "yellowtail", "yellowthroat", "yuhina"},
		'z': {"zander", "zebra", "zebradove", "zebrafinch", "zebrafish", "zebu", "zeren", "zokor", "zooplankton", "zorilla", "zorro"},
	},

	"minerals": {
		'a': {"agate", "alabaster", "albite", "alexandrite", "almandine", "amazonite", "amber", "amethyst", "analcime", "anatase", "andalusite", "andradite", "anglesite", "anhydrite", "ankerite", "annabergite", "anorthite", "antigorite", "apatite", "apophyllite", "aquamarine", "aragonite", "arsenopyrite", "atacamite", "augite", "autunite", "axinite", "azurite"},
		'b': {"babingtonite", "baddeleyite", "barite", "bastnasite", "bauxite", "benitoite", "berthierite", "beryl", "beudantite", "biotite", "bismuthinite", "bloodstone", "boehmite", "boleite", "borax", "bornite", "boulangerite", "bournonite", "brazilianite", "brochantite", "bromellite", "bronzite", "brookite", "brucite", "bustamite", "bytownite"},
		'c': {"calcite", "cancrinite", "carnelian", "carnotite", "cassiterite", "celestine", "cerussite", "chabazite", "chalcedony", "chalcocite", "chalcopyrite", "charoite", "chlorite", "chromite", "chrysoberyl", "chrysocolla", "chrysoprase", "cinnabar", "citrine", "clinochlore", "cobaltite", "coesite", "colemanite", "columbite", "cordierite", "corundum", "covellite", "crocoite", "cryolite", "cuprite"},
		'd': {"danburite", "datolite", "dawsonite", "descloizite", "diamond", "diaspore", "dickite", "digenite", "diopside", "dioptase", "djurleite", "dolomite", "domeykite", "dravite", "duftite", "dumortierite", "dundasite", "dyscrasite"},
		'e': {"eckermannite", "edenite", "edingtonite", "eglestonite", "elbaite", "elpidite", "emerald", "emplectite", "enargite", "enstatite", "eosphorite", "epidote", "epsomite", "erionite", "erythrite", "esperite", "ettringite", "euclase", "eudialyte", "euxenite"},
		'f': {"famatinite", "farringtonite", "faujasite", "fayalite", "feldspar", "ferberite", "fergusonite", "ferrierite", "ferrihydrite", "fizelyite", "flagstaffite", "florencite", "fluellite", "fluoborite", "fluorapatite", "fluorite", "forsterite", "franklinite", "freibergite", "fuchsite"},
		'g': {"gadolinite", "gahnite", "galena", "garnet", "gaylussite", "gedrite", "geikielite", "gersdorffite", "gibbsite", "glauberite", "glaucophane", "gmelinite", "goethite", "gold", "goslarite", "graphite", "greenockite", "grossular", "gypsum", "gyrolite"},
		'h': {"halite", "hambergite", "hanksite", "hauerite", "hausmannite", "hedenbergite", "heliodor", "hematite", "hemimorphite", "herderite", "hessonite", "heulandite", "hiddenite", "hornblende", "howlite", "hubnerite", "humite", "huntite", "hureaulite", "hydrozincite"},
		'i': {"ianthinite", "icosahedrite", "iddingsite", "idocrase", "illite", "ilmenite", "ilsemannite", "ilvaite", "imogolite", "inderite", "indialite", "inesite", "inyoite", "iodargyrite", "iolite", "iranite"},
		'j': {"jade", "jadeite", "jagoite", "jalpaite", "jamesonite", "jarlite", "jarosite", "jasper", "jeffreyite", "jennite", "jeremejevite", "jervisite", "jimthompsonite", "joaquinite", "johannsenite", "jordanite", "julgoldite", "junitoite"},
		'k': {"kaersutite", "kainite", "kalsilite", "kaolinite", "karelianite", "kasolite", "katophorite", "kernite", "kesterite", "khatyrkite", "kimzeyite", "kinoite", "klockmannite", "kobellite", "koenenite", "kornerupine", "kunzite", "kutnohorite", "kyanite", "kyzylkumite"},
		'l': {"labradorite", "lanarkite", "langbeinite", "larnite", "laumontite", "laurionite", "lawsonite", "lazulite", "lazurite", "legrandite", "lepidolite", "leucite", "libethenite", "linarite", "lizardite", "lollingite", "lonsdaleite", "lorandite", "ludlamite", "luzonite"},
		'm': {"magnesite", "magnetite", "malachite", "manganite", "marcasite", "margarite", "massicot", "melanterite", "mellite", "mesolite", "miargyrite", "microcline", "millerite", "mimetite", "minium", "mirabilite", "moissanite", "molybdenite", "monazite", "montmorillonite", "mordenite", "morganite", "mottramite", "muscovite"},
		'n': {"nacrite", "nadorite", "nagyagite", "nambulite", "narsarsukite", "natrojarosite", "natrolite", "naumannite", "nepheline", "nephrite", "neptunite", "niccolite", "nickeline", "niobite", "nitratine", "nontronite", "norbergite", "nosean", "nsutite"},
		'o': {"offretite", "okanoganite", "okenite", "oldhamite", "olivenite", "olivine", "olmiite", "omphacite", "onyx", "oosterboschite", "opal", "orcelite", "orpiment", "orthoclase", "osarizawaite", "osmium", "otavite", "ottrelite", "ourayite", "oyelite"},
		'p': {"painite", "paragonite", "pargasite", "pectolite", "pentlandite", "periclase", "peridot", "perovskite", "petalite", "phenakite", "phlogopite", "phosgenite", "piemontite", "pigeonite", "pinnoite", "plagioclase", "platinum", "pollucite", "polybasite", "prehnite", "proustite", "pumpellyite", "pyrargyrite", "pyrite", "pyrolusite", "pyromorphite", "pyrope", "pyrophyllite", "pyrrhotite"},
		'q': {"qandilite", "qilianshanite", "qingsongite", "quadratite", "quadridavyne", "quadruphite", "quartz", "queitite", "quenselite", "quenstedtite", "quetzalcoatlite", "quijarroite", "quintinite"},
		'r': {"rammelsbergite", "rankinite", "raspite", "rathite", "realgar", "retgersite", "rhodochrosite", "rhodolite", "rhodonite", "rhomboclase", "richterite", "riebeckite", "ripidolite", "romanechite", "rosasite", "roscoelite", "rubellite", "ruby", "rutile"},
		's': {"sanidine", "sapphire", "sarcolite", "scapolite", "scheelite", "schorl", "scolecite", "selenite", "sepiolite", "serandite", "serpentine", "siderite", "sillimanite", "silver", "skutterudite", "smithsonite", "sodalite", "spessartine", "sphalerite", "sphene", "spinel", "spodumene", "staurolite", "stibnite", "stilbite", "stishovite", "strontianite", "sugilite", "sulfur", "sylvanite"},
		't': {"talc", "tantalite", "tanzanite", "tellurite", "tennantite", "tenorite", "tephroite", "tetrahedrite", "thaumasite", "thenardite", "thomsonite", "thorianite", "thulite", "titanite", "topaz", "torbernite", "tourmaline", "tremolite", "tridymite", "troilite", "tsavorite", "tugtupite", "turquoise", "tyuyamunite"},
		'u': {"uklonskovite", "ulexite", "ullmannite", "ulrichite", "ulvospinel", "umangite", "umbite", "ungemachite", "uraninite", "uranocircite", "uranopilite", "uranophane", "ussingite", "uvarovite", "uvite", "uytenbogaardtite"},
		'v': {"vaesite", "valentinite", "valleriite", "vanadinite", "vandenbrandeite", "vanthoffite", "variscite", "vashegyite", "vaterite", "vauxite", "vermiculite", "verplanckite", "vesuvianite", "veszelyite", "villiaumite", "violarite", "vivianite", "vlasovite", "volborthite", "voltaite"},
		'w': {"wad", "walpurgite", "wardite", "warwickite", "wavellite", "weeksite", "wegscheiderite", "weloganite", "wernerite", "westerveldite", "whewellite", "whitlockite", "willemite", "witherite", "wolframite", "wollastonite", "wulfenite", "wurtzite", "wustite"},
		'x': {"xanthoconite", "xanthiosite", "xanthophyllite", "xenophyllite", "xenotime", "xieite", "xifengite", "xiangjiangite", "xilingolite", "xingzhongite", "xocolatlite", "xocomecatlite", "xonotlite"},
		'y': {"yangite", "yarlongite", "yaroshevskite", "yavapaiite", "yeatmanite", "yedlinite", "yimengite", "yoderite", "yofortierite", "yttrialite", "yttrocerite", "yttrotantalite", "yugawaralite", "yuksporite"},
		'z': {"zaratite", "zektzerite", "zemannite", "zeolite", "zeunerite", "zhanghengite", "zinalsite", "zincite", "zinkenite", "zinnwaldite", "zippeite", "zircon", "zirkelite", "zoisite", "zoubekite", "zunyite", "zussmanite", "zykaite"},
	},

	"cities": {
		'a': {"abidjan", "abuja", "accra", "adelaide", "aden", "agra", "ahmedabad", "albuquerque", "aleppo", "alexandria", "algiers", "almaty", "amman", "amsterdam", "anchorage", "ankara", "antwerp", "aomori", "arequipa", "asheville", "ashgabat", "asmara", "astana", "asuncion", "athens", "atlanta", "auckland", "augsburg", "austin"},
		'b': {"baghdad", "baku", "baltimore", "bamako", "bandung", "bangalore", "bangkok", "banjul", "barcelona", "basel", "basra", "beirut", "belfast", "belgrade", "bergen", "berlin", "bern", "bilbao", "birmingham", "bishkek", "bissau", "bogota", "bologna", "bonn", "bordeaux", "boston", "brasilia", "bratislava", "brisbane", "bristol", "brno", "brussels", "bucharest", "budapest", "bujumbura", "busan"},
		'c': {"cairo", "calgary", "cali", "campinas", "canberra", "cancun", "capetown", "caracas", "cardiff", "cartagena", "casablanca", "catania", "cebu", "changsha", "chattanooga", "chelyabinsk", "chengdu", "chennai", "chicago", "chisinau", "chittagong", "chongqing", "cincinnati", "cleveland", "cologne", "colombo", "conakry", "copenhagen", "cordoba", "cork", "cotonou", "coventry", "cuenca", "curitiba", "cusco"},
		'd': {"dakar", "dalian", "dallas", "damascus", "danang", "darwin", "davao", "dayton", "delhi", "denpasar", "denver", "derby", "detroit", "dhaka", "dijon", "dili", "djibouti", "dnipro", "doha", "donetsk", "dortmund", "douala", "dover", "dresden", "dubai", "dublin", "dubrovnik", "duluth", "dunedin", "durban", "durham", "dushanbe", "dusseldorf"},
		'e': {"edinburgh", "edmonton", "eindhoven", "elpaso", "enschede", "entebbe", "erbil", "erfurt", "erie", "esbjerg", "escondido", "esfahan", "essen", "eugene", "eureka", "evansville", "exeter"},
		'f': {"fairbanks", "faisalabad", "fargo", "faro", "fes", "flagstaff", "florence", "florianopolis", "fortaleza", "foshan", "frankfurt", "freetown", "fresno", "fujairah", "fukuoka", "funchal", "fuzhou"},
		'g': {"gaborone", "galway", "gaza", "gdansk", "geneva", "genoa", "ghent", "gibraltar", "giza", "glasgow", "goiania", "gothenburg", "granada", "graz", "greensboro", "grenoble", "groningen", "guadalajara", "guangzhou", "guatemala", "guayaquil", "guilin", "guiyang", "gwangju", "gyor"},
		'h': {"haifa", "haiphong", "hakodate", "halifax", "hamburg", "hamilton", "hangzhou", "hanoi", "harare", "harbin", "hartford", "havana", "heidelberg", "helsinki", "hermosillo", "hiroshima", "hobart", "hohhot", "honolulu", "houston", "huambo", "hue", "hull", "huntsville", "hyderabad"},
		'i': {"ibadan", "ibiza", "iasi", "iloilo", "incheon", "indianapolis", "indore", "innsbruck", "inverness", "ipoh", "iquitos", "irbid", "irkutsk", "isfahan", "islamabad", "istanbul", "izhevsk", "izmir"},
		'j': {"jabalpur", "jackson", "jaipur", "jakarta", "jalandhar", "jamshedpur", "jeddah", "jena", "jerusalem", "jilin", "jinan", "jincheng", "jiujiang", "jodhpur", "johannesburg", "joinville", "jos", "juba", "juneau", "jyvaskyla"},
		'k': {"kabul", "kampala", "kandy", "kano", "karachi", "kathmandu", "kaunas", "kazan", "kharkiv", "khartoum", "kiel", "kigali", "kingston", "kinshasa", "kobe", "kolkata", "krakow", "kuching", "kumasi", "kunming", "kuwait", "kyiv", "kyoto"},
		'l': {"lagos", "lahore", "lanzhou", "laredo", "lausanne", "leeds", "leipzig", "leon", "lhasa", "liege", "lille", "lima", "limerick", "linz", "lisbon", "liverpool", "ljubljana", "lodz", "lome", "london", "louisville", "luanda", "lubbock", "lublin", "lucknow", "lusaka", "luxembourg", "lviv", "lyon"},
		'm': {"maastricht", "macau", "madrid", "malaga", "malmo", "managua", "manaus", "manchester", "manila", "mannheim", "maputo", "maracaibo", "marrakesh", "marseille", "maseru", "mashhad", "medan", "medellin", "melbourne", "memphis", "mendoza", "merida", "messina", "mexicali", "miami", "milan", "milwaukee", "minsk", "mogadishu", "mombasa", "monterrey", "montevideo", "montreal", "moscow", "mosul", "mumbai", "munich", "muscat"},
		'n': {"nagasaki", "nagoya", "nagpur", "nairobi", "nanchang", "nanjing", "nanning", "nantes", "naples", "nashville", "nassau", "natal", "ndjamena", "newark", "newcastle", "niamey", "nicosia", "nijmegen", "ningbo", "norfolk", "nottingham", "nouakchott", "novosibirsk", "nuremberg", "nuuk"},
		'o': {"oakland", "oaxaca", "odense", "odesa", "ogden", "okayama", "okinawa", "oklahoma", "oldenburg", "olomouc", "omaha", "omsk", "oporto", "oran", "orlando", "osaka", "oslo", "ostrava", "ottawa", "ouagadougou", "oulu", "oviedo", "oxford"},
		'p': {"padua", "palembang", "palermo", "palma", "panama", "paramaribo", "paris", "patna", "pattaya", "pecs", "penang", "perm", "perth", "peshawar", "philadelphia", "phnompenh", "phoenix", "pilsen", "pittsburgh", "plovdiv", "plymouth", "podgorica", "pontianak", "poznan", "prague", "pretoria", "pristina", "providence", "puebla", "pune", "pyongyang"},
		'q': {"qazvin", "qena", "qingdao", "qinhuangdao", "qiqihar", "qom", "quakertown", "quanzhou", "quebec", "quelimane", "queretaro", "quetta", "quezon", "quilmes", "quilon", "quilpue", "quimper", "quito", "qujing", "quzhou"},
		'r': {"rabat", "raipur", "raleigh", "ramallah", "ranchi", "rawalpindi", "reading", "recife", "regensburg", "reggio", "regina", "reims", "rennes", "reno", "reykjavik", "richmond", "riga", "rijeka", "rimini", "riyadh", "rochester", "rosario", "rostock", "rotterdam", "rouen", "rovaniemi"},
		's': {"sacramento", "salvador", "salzburg", "samara", "sanaa", "santiago", "santos", "sapporo", "sarajevo", "saskatoon", "seattle", "semarang", "sendai", "seoul", "seville", "shanghai", "sharjah", "sheffield", "shenyang", "shenzhen", "shiraz", "shizuoka", "siena", "singapore", "skopje", "sofia", "sokoto", "split", "srinagar", "stavanger", "stockholm", "strasbourg", "stuttgart", "surabaya", "surat", "suva", "sydney", "szeged"},
		't': {"tabriz", "taichung", "taipei", "taiyuan", "tallinn", "tampere", "tangier", "tashkent", "tbilisi", "tegucigalpa", "tehran", "tianjin", "tijuana", "tilburg", "timisoara", "tirana", "tokyo", "toledo", "toluca", "tomsk", "toronto", "toulouse", "townsville", "trieste", "tripoli", "trondheim", "tucson", "tulsa", "tunis", "turin", "turku"},
		'u': {"ube", "uberaba", "uberlandia", "udaipur", "udine", "ufa", "uige", "ujjain", "ulaanbaatar", "ulm", "ulsan", "umea", "uppsala", "urfa", "uruapan", "urumqi", "usak", "ushuaia", "utrecht", "utsunomiya", "uvira", "uyo"},
		'v': {"vaasa", "vadodara", "valencia", "valletta", "valparaiso", "vancouver", "varanasi", "varna", "venice", "veracruz", "verona", "vicenza", "victoria", "vienna", "vientiane", "vigo", "vijayawada", "vilnius", "visakhapatnam", "vitoria", "vladivostok", "volgograd", "voronezh"},
		'w': {"waco", "warsaw", "washington", "waterford", "weifang", "wellington", "wenling", "wenzhou", "wichita", "wiesbaden", "windhoek", "winnipeg", "wollongong", "worcester", "wroclaw", "wuhan", "wuppertal", "wurzburg", "wuxi"},
		'x': {"xalapa", "xankendi", "xiamen", "xian", "xiangtan", "xiangyang", "xianyang", "xiaogan", "xichang", "xilinhot", "xingtai", "xinghua", "xining", "xinxiang", "xinyang", "xinyu", "xinzhou", "xuchang", "xuzhou"},
		'y': {"yakutsk", "yamoussoukro", "yancheng", "yangon", "yanji", "yantai", "yaounde", "yazd", "yekaterinburg", "yerevan", "yibin", "yichang", "yingkou", "yiwu", "yogyakarta", "yokohama", "yokosuka", "york", "yueyang", "yuma"},
		'z': {"zaanstad", "zadar", "zagreb", "zahedan", "zalau", "zamboanga", "zanzibar", "zaporizhzhia", "zaragoza", "zhanjiang", "zhengzhou", "zhuhai", "zhuzhou", "zibo", "zigong", "zinder", "zomba", "zunyi", "zurich", "zvolen", "zwolle"},
	},

	"rivers": {
		'a': {"aare", "alabama", "allegheny", "amazon", "amu", "amur", "angara", "apure", "araguaia", "aras", "arkansas", "arno", "aruwimi", "atbara", "athabasca", "avoca", "avon"},
		'b': {"bandama", "barito", "beni", "benue", "bermejo", "betsiboka", "bhima", "blackwater", "boyne", "brahmani", "brahmaputra", "brazos", "brenta", "bug", "buller", "burdekin"},
		'c': {"cauca", "cavally", "charente", "chari", "chenab", "cher", "chindwin", "chubut", "churchill", "clutha", "clyde", "coco", "colorado", "columbia", "congo", "connecticut", "cooper", "coppermine", "cuanza"},
		'd': {"danube", "darling", "daugava", "dee", "delaware", "demerara", "derwent", "desna", "dnieper", "dniester", "don", "donets", "dordogne", "doubs", "douro", "drava", "drina", "dvina"},
		'e': {"ebro", "elbe", "elwha", "embarras", "ems", "erne", "escambia", "esla", "essequibo", "etowah", "euphrates", "exe"},
		'f': {"feather", "findhorn", "fitzroy", "flinders", "flint", "fly", "forth", "foyle", "fraser", "frome", "fuchun"},
		'g': {"gambia", "ganges", "garonne", "gila", "glomma", "godavari", "gogra", "gomti", "goulburn", "grande", "green", "guadalquivir", "guadiana", "guaviare", "gudena", "gurupi"},
		'h': {"han", "hawkesbury", "helmand", "hooghly", "huallaga", "hudson", "humber", "hunter", "hunza"},
		'i': {"iguazu", "ilek", "illinois", "indigirka", "indre", "indus", "inn", "ipel", "iriri", "irrawaddy", "irtysh", "isere", "ishim", "itapicuru"},
		'j': {"jachal", "jaguaribe", "james", "jamuna", "japura", "jequitinhonha", "jhelum", "jialing", "jinsha", "jizera", "jordan", "jucar", "jurua"},
		'k': {"kafue", "kagera", "kama", "kapuas", "kasai", "kaveri", "ken", "kennebec", "kizilirmak", "klamath", "kolyma", "kootenay", "krishna", "kuban", "kura", "kuskokwim", "kwango", "kymi"},
		'l': {"lachlan", "lagan", "lek", "lena", "liao", "liffey", "limpopo", "logone", "loire", "lomami", "lot", "lualaba", "luangwa", "luapula", "lule"},
		'm': {"mackenzie", "madeira", "magdalena", "mahanadi", "main", "manas", "maranon", "marne", "mekong", "merrimack", "meuse", "minho", "mississippi", "missouri", "mobile", "moselle", "mures", "murray", "murrumbidgee", "muskingum"},
		'n': {"namoi", "napo", "narmada", "nechako", "neches", "neckar", "negro", "nelson", "neman", "nen", "neva", "nidd", "niger", "nile", "nith", "nueces", "nushagak"},
		'o': {"ob", "ocmulgee", "oconee", "oder", "ogooue", "ohio", "oka", "okavango", "olt", "omo", "onega", "orange", "ord", "orinoco", "orontes", "ouse", "oyapock"},
		'p': {"paraguay", "paraiba", "parana", "parnaiba", "passaic", "peace", "pearl", "pechora", "pecos", "penobscot", "pilcomayo", "platte", "po", "potomac", "pripyat", "prut", "purus", "putumayo"},
		'q': {"quaboag", "queets", "quesnel", "quilcene", "quinapoxet", "quinault", "quinebaug", "quinnipiac", "quittapahilla", "quoich"},
		'r': {"raritan", "red", "rhine", "rhone", "ribble", "rideau", "rio", "rioni", "roanoke", "rogue", "ruhr", "rufiji", "ruki", "rupert", "russian", "ruvuma"},
		's': {"saale", "saguenay", "salween", "sambre", "san", "sanaga", "saskatchewan", "sava", "savannah", "scheldt", "seine", "sepik", "severn", "shannon", "shire", "sittang", "somme", "songhua", "spey", "sungari", "susquehanna", "sutlej", "syr"},
		't': {"tagus", "tamar", "tana", "tapajos", "tarim", "tees", "tejo", "tennessee", "thames", "thjorsa", "tiber", "ticino", "tigris", "tisza", "tocantins", "tombigbee", "tone", "torne", "trent", "trinity", "tugela", "tumen", "tyne"},
		'u': {"uatuma", "ubangi", "ucayali", "uda", "uele", "ulua", "ume", "umpqua", "unzha", "ural", "uruguay", "ussuri", "usumacinta"},
		'v': {"vaal", "vah", "var", "vardar", "verde", "vienne", "vilyuy", "vistula", "vitim", "vltava", "volga", "volkhov", "volta", "vyatka", "vychegda"},
		'w': {"wabash", "waikato", "wami", "warta", "waveney", "wear", "welland", "weser", "white", "willamette", "witham", "wupper", "wye"},
		'x': {"xallas", "xanthos", "xi", "xiang", "xiaoqing", "xijiang", "xiliao", "xinanjiang", "xingu", "xucar", "xun"},
		'y': {"yadkin", "yalong", "yalu", "yamuna", "yangtze", "yaqui", "yarkand", "yarra", "yegorlyk", "yellowstone", "yenisei", "yonne", "yser", "yuba", "yukon"},
		'z': {"zab", "zaire", "zambezi", "zanskar", "zarqa", "zayandeh", "zenne", "zeya", "zhem", "zschopau", "zuari"},
	},

	"trees": {
		'a': {"acacia", "ailanthus", "alder", "allspice", "almond", "angophora", "apple", "apricot", "araucaria", "arbutus", "ash", "aspen", "avocado"},
		'b': {"balsa", "bamboo", "banyan", "baobab", "basswood", "bay", "beech", "birch", "blackthorn", "blackwood", "bloodwood", "boxelder", "brazilwood", "breadfruit", "buckeye", "butternut"},
		'c': {"camphor", "candlenut", "carob", "cashew", "cassia", "catalpa", "cedar", "ceiba", "cherry", "chestnut", "chinaberry", "cinnamon", "citron", "coconut", "coolibah", "cottonwood", "crabapple", "cypress"},
		'd': {"dahoon", "dalbergia", "damson", "date", "deodar", "dogwood", "douglas", "dragonblood", "durian"},
		'e': {"ebony", "elder", "elm", "empress", "eucalyptus", "euonymus", "eugenia"},
		'f': {"fig", "filbert", "fir", "flamboyant", "frangipani", "franklinia", "fustic"},
		'g': {"ginkgo", "gmelina", "goldenrain", "greenheart", "guava", "gum", "gutta"},
		'h': {"hackberry", "hawthorn", "hazel", "hemlock", "hickory", "holly", "honeylocust", "hornbeam", "horsechestnut"},
		'i': {"ilex", "inkwood", "ipe", "ironbark", "ironwood"},
		'j': {"jacaranda", "jackfruit", "jarrah", "jatoba", "jelutong", "judas", "jujube", "juniper"},
		'k': {"kapok", "karri", "katsura", "kauri", "kiri", "koa", "kowhai", "kumquat"},
		'l': {"laburnum", "larch", "laurel", "leadwood", "lemon", "lilac", "lime", "linden", "logwood", "longan", "lychee"},
		'm': {"macadamia", "madrone", "magnolia", "mahogany", "mango", "mangrove", "manuka", "maple", "marula", "mesquite", "mimosa", "mopane", "mulberry", "myrtle"},
		'n': {"nectarine", "neem", "nutmeg", "nyssa"},
		'o': {"oak", "olive", "osage", "osier"},
		'p': {"padauk", "palm", "papaya", "pawpaw", "peach", "pear", "pecan", "persimmon", "pine", "pistachio", "plane", "plum", "podocarp", "pomegranate", "poplar", "purpleheart"},
		'q': {"quandong", "quararibea", "quassia", "quebracho", "quillaja", "quince"},
		'r': {"rambutan", "redbud", "redwood", "rimu", "rosewood", "rowan", "rubber"},
		's': {"sandalwood", "sapele", "sassafras", "satinwood", "sequoia", "serviceberry", "sourwood", "spruce", "sumac", "sweetgum", "sycamore"},
		't': {"tallow", "tamarack", "tamarind", "teak", "thuja", "totara", "tulip", "tupelo"},
		'u': {"ucuuba", "ulmus", "umbrella"},
		'v': {"varnish", "viburnum", "vitex"},
		'w': {"walnut", "wattle", "wenge", "willow", "witchhazel"},
		'x': {"xanthoceras", "xanthostemon", "ximenia", "xylosma"},
		'y': {"yellowwood", "yew", "ylang", "yohimbe"},
		'z': {"zebrawood", "zelkova", "ziziphus"},
	},

	"stars": {
		'a': {"acamar", "achernar", "achird", "acrab", "acrux", "acubens", "adhafera", "adhara", "ain", "aladfar", "albali", "albireo", "alchiba", "alcor", "alcyone", "aldebaran", "alderamin", "alfirk", "algedi", "algenib", "algieba", "algol", "algorab", "alhena", "alioth", "alkaid", "alkes", "almaaz", "almach", "alnair", "alnasl", "alnilam", "alnitak", "alphard", "alphecca", "alpheratz", "alrescha", "alsafi", "alshain", "altair", "altais", "alterf", "aludra", "alula", "alya", "alzirr", "ancha", "ankaa", "antares", "arcturus", "arkab", "arneb", "ascella", "asellus", "aspidiske", "asterope", "atik", "atlas", "atria", "avior", "azelfafage", "azha", "azmidi"},
		'b': {"baten", "bellatrix", "betelgeuse", "biham", "botein", "brachium", "bunda"},
		'c': {"canopus", "capella", "caph", "castor", "cebalrai", "celaeno", "chalawan", "chamukuy", "chara", "chertan", "copernicus", "cujam", "cursa"},
		'd': {"dabih", "dalim", "deneb", "denebola", "diadem", "diphda", "dschubba", "dubhe", "dziban"},
		'e': {"edasich", "electra", "elgafar", "elkurud", "elnath", "eltanin", "enif", "errai"},
		'f': {"fafnir", "fang", "fawaris", "felis", "fomalhaut", "fulu", "fumalsamakah", "furud"},
		'g': {"gacrux", "gienah", "ginan", "gomeisa", "grumium", "gudja", "gumala"},
		'h': {"hadar", "haedus", "hamal", "hassaleh", "hatysa", "helvetios", "heze", "homam"},
		'i': {"iklil", "imai", "intercrus", "izar"},
		'j': {"jabbah", "jishui"},
		'k': {"kaffaljidhma", "kakkab", "kang", "kaus", "keid", "khambalia", "kitalpha", "kochab", "kornephoros", "kraz", "kurhah"},
		'l': {"larawag", "lesath", "libertas", "lich", "lilii", "lucilinburhuc", "lusitania"},
		'm': {"maasym", "mahasim", "maia", "marfik", "markab", "marsic", "matar", "mebsuta", "megrez", "meissa", "mekbuda", "menkalinan", "menkar", "menkent", "menkib", "merak", "merga", "meridiana", "merope", "mesarthim", "miaplacidus", "minelauva", "mintaka", "mira", "mirach", "miram", "mirfak", "mirzam", "mizar", "mothallah", "muliphein", "muphrid", "musica"},
		'n': {"naos", "nashira", "nekkar", "nembus", "nihal", "nunki", "nusakan"},
		'o': {"ogma", "okab", "orkaria"},
		'p': {"paikauhale", "phact", "phecda", "pherkad", "piautos", "pipirima", "pleione", "polaris", "pollux", "porrima", "praecipua", "prima", "procyon", "propus"},
		'q': {},
		'r': {"rana", "rasalas", "rasalgethi", "rasalhague", "rastaban", "regulus", "revati", "rigel", "rotanev", "ruchbah", "rukbat"},
		's': {"sabik", "sadachbia", "sadalbari", "sadalmelik", "sadalsuud", "sadr", "saiph", "salm", "sargas", "sarin", "sceptrum", "scheat", "schedar", "secunda", "segin", "seginus", "shaula", "sheliak", "sheratan", "sirius", "situla", "skat", "spica", "sualocin", "subra", "suhail", "sulafat", "syrma"},
		't': {"tabit", "taiyangshou", "talitha", "tania", "tarazed", "tarf", "taygeta", "tegmine", "tejat", "terebellum", "theemin", "thuban", "tiaki", "tianguan", "titawin", "toliman", "torcular", "tureis"},
		'u': {"ukdah", "unukalhai", "unurgunite"},
		'v': {"vega", "veritate", "vindemiatrix"},
		'w': {"wasat", "wazn", "wezen", "wurren"},
		'x': {"xamidimura", "xihe", "xuange"},
		'y': {"yed", "yildun"},
		'z': {"zaniah", "zaurak", "zavijava", "zhang", "zibal", "zosma", "zubenelgenubi", "zubenelhakrabi", "zubeneschamali"},
	},
}

// letterPools is the flattened view pools is actually read through: one
// ordered word list per letter, namespaces concatenated in
// namespaceOrder. Built once, at first use.
var letterPools = func() map[byte][]string {
	flat := make(map[byte][]string, len(alphabet))
	for _, letter := range alphabet {
		var words []string
		for _, ns := range namespaceOrder {
			words = append(words, pools[ns][letter]...)
		}
		flat[letter] = words
	}
	return flat
}()

// PoolWord returns the n'th codename (0-based) for letter, drawing
// animals before minerals before cities before rivers before trees
// before stars. Past the end of the real words it wraps and appends the
// lap number -- quail, quokka, ..., quoll, quail2, quokka2, ... -- so
// every letter is effectively bottomless and no letter ever has to be
// skipped for want of a word. Pure arithmetic: a slice index, a modulo
// and a division, with no model in the path.
func PoolWord(letter byte, n int) string {
	words := letterPools[letter]
	if len(words) == 0 || n < 0 {
		return ""
	}
	word := words[n%len(words)]
	if lap := n / len(words); lap > 0 {
		word = fmt.Sprintf("%s%d", word, lap+1)
	}
	return word
}

// LetterDepth is how many real (unsuffixed) words a letter has across
// all six namespaces -- the point at which PoolWord starts suffixing.
func LetterDepth(letter byte) int { return len(letterPools[letter]) }

// NamespaceDepth is how many real words a namespace carries for a
// letter. Reporting only; the claim path reads letterPools.
func NamespaceDepth(namespace string, letter byte) int {
	return len(pools[namespace][letter])
}

// PrintPoolDepth writes the per-letter, per-namespace word counts as one
// tab-separated table -- machine-first, same house style as the rest of
// the CLI. "depth" is the number of real words before PoolWord starts
// suffixing; it is a capacity figure, not a limit, since every letter is
// bottomless by construction.
func PrintPoolDepth(w io.Writer) {
	fmt.Fprintf(w, "letter\t%s\ttotal\n", strings.Join(namespaceOrder, "\t"))
	totals := make(map[string]int, len(namespaceOrder))
	grand := 0
	for _, letter := range alphabet {
		fmt.Fprintf(w, "%c", letter)
		for _, ns := range namespaceOrder {
			n := NamespaceDepth(ns, letter)
			totals[ns] += n
			fmt.Fprintf(w, "\t%d", n)
		}
		grand += LetterDepth(letter)
		fmt.Fprintf(w, "\t%d\n", LetterDepth(letter))
	}
	fmt.Fprint(w, "total")
	for _, ns := range namespaceOrder {
		fmt.Fprintf(w, "\t%d", totals[ns])
	}
	fmt.Fprintf(w, "\t%d\n", grand)
}
