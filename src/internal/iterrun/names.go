// Package iterrun: plan codename assignment. /iterate-planner and /iterate
// used to have the LLM "pick a random common animal not already present in
// plans/" — a per-project check only, so nothing stopped two different
// projects from independently picking the same word (confirmed live:
// "wren" got used by two unrelated projects, and see the dashboard bugs
// that same collision caused). NextPlanName replaces that with: each
// project walks the alphabet on its OWN sequence (that project's 1st new
// plan is an a-word, 2nd is a b-word, ...), while the actual word chosen
// for a letter is drawn from one machine-wide "already used" set — so two
// projects' first plans are both a-words, just never the SAME a-word
// (project 1 gets "aardvark", project 2's first plan skips it for the next
// available a-word, "antelope" — it does not jump ahead to a b-word).
package iterrun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// animalsByLetter is the pool NextPlanName draws from, grouped by first
// letter so assignment can walk the alphabet in order. Every letter has at
// least one entry; letters with few common one-word animal names (q, u, x,
// y) just have a smaller pool, which only matters once that letter comes
// up for the Nth time in the a-through-z cycle.
var animalsByLetter = map[byte][]string{
	'a': {"aardvark", "antelope", "ant", "ape", "auk", "alpaca", "addax", "agouti", "akita", "albatross", "alligator", "anaconda", "anchovy", "angelfish", "anhinga", "ani", "anoa", "anole", "anteater", "antechinus", "aoudad", "aphid", "apollo", "arapaima", "argali", "armadillo", "asp", "avocet", "axolotl", "aye", "azurevireo", "abalone", "acouchi", "adder", "aegagrus", "aardwolf", "albacore", "alewife", "amberjack", "ammonite", "anemone", "angwantibo", "anableps", "abudefduf", "acanthisitta", "accentor", "acorn", "adjutant", "aegithalos", "aeshna", "affenpinscher", "agama", "aardvarkcucumber", "ailuropoda", "airedale", "ajolote", "akepa", "alaskapollock", "albertosaurus", "alcid", "alderfly", "aleutiantern", "algaeeater", "allenshummingbird", "allosaurus", "alpineibex", "amazonparrot", "ambushbug", "ammodytes", "amoeba", "amphiuma", "anchovypear", "andeancondor", "angelshark", "anglerfish", "angora", "anhingidae"},
	'b': {"badger", "bear", "bison", "boar", "bobcat", "bee", "babirusa", "baboon", "bandicoot", "barasingha", "barbet", "barnacle", "barracuda", "basilisk", "bass", "bat", "bateleur", "beagle", "bearcat", "beaver", "beetle", "beluga", "bettong", "bharal", "bichir", "bilby", "binturong", "bittern", "blackbird", "blesbok", "bluebird", "bluegill", "boa", "bobolink", "bongo", "bonobo", "booby", "bowerbird", "brambling", "bream", "brolga", "budgerigar", "buffalo", "bulbul", "bullfinch", "bunting", "burbot", "bushbaby", "bustard", "butterfly", "buzzard", "babbler", "bactrian", "bagworm", "baiji", "balaena", "baldeagle", "baleenwhale", "bananaslug", "bankvole", "bantam", "barbary", "barbel", "barbetbird", "bargoose", "barnowl", "barnswallow", "barredowl", "basenji", "basilosaurus", "basking", "bassethound", "batfish", "bathynomus", "beardeddragon", "bearded", "bedbug", "beeeater", "beefalo"},
	'c': {"cat", "crow", "cobra", "coyote", "crane", "civet", "caiman", "camel", "capybara", "caracal", "cardinal", "caribou", "carp", "cassowary", "caterpillar", "catfish", "cavy", "centipede", "chaffinch", "chameleon", "chamois", "cheetah", "chevrotain", "chickadee", "chicken", "chimpanzee", "chinchilla", "chipmunk", "chiton", "chough", "chub", "cicada", "cichlid", "clam", "coati", "cockatoo", "cod", "coelacanth", "colobus", "condor", "conger", "coot", "cormorant", "cougar", "cowbird", "coypu", "crab", "crayfish", "cricket", "crocodile", "crossbill", "cuckoo", "curlew", "cuscus", "cuttlefish", "cachalot", "caddisfly", "caecilian", "calandra", "calfbird", "californiacondor", "canvasback", "capelin", "capercaillie", "capuchin", "caracara", "carcharodon", "cariama", "carpenterbee", "cattleegret", "cavefish", "cedarwaxwing", "centrolene", "cephalopod", "cerulean"},
	'd': {"dog", "deer", "dove", "dolphin", "duck", "donkey", "dace", "dachshund", "damselfly", "darter", "dassie", "dhole", "dibatag", "dikdik", "dingo", "dipper", "discus", "dogfish", "dormouse", "dotterel", "dourocouli", "dowitcher", "dragonet", "dragonfly", "drever", "dromedary", "drongo", "dugong", "dunlin", "dunnart", "dunnock", "dusky", "dziggetai", "dartfrog", "degu", "desman", "devil", "diver", "dolphinfish", "dorado", "douc", "dragon", "drill", "dromaius", "dabchick", "dalmatian", "damaraland", "darklingbeetle", "dartertail", "dasyure", "deathwatch", "deerhound", "demoiselle", "dendrobates", "devilfish", "diamondback", "dickcissel", "dikkop", "dinosaur", "diplodocus", "dipnoi", "discusfish", "diverbird", "dobsonfly", "dodo", "dollarbird", "dolphinsafe", "donacobius", "dorcasgazelle", "dormancy", "dracaena", "dragonlizard", "drillmonkey", "dromiceius", "drumfish", "dryad"},
	'e': {"elk", "eagle", "egret", "eel", "emu", "earthworm", "earwig", "echidna", "eclectus", "eider", "eland", "elephant", "elephantseal", "elver", "emerald", "emperor", "ermine", "escolar", "eulachon", "euphausia", "eyra", "eagleray", "eartheater", "echinoderm", "eft", "egg", "ekaltadeta", "elanus", "elephantfish", "elkhound", "elopidae", "emperorpenguin", "enhydra", "ensatina", "eonycteris", "epaulette", "eagleowl", "earlgrey", "earthsnake", "easternbluebird", "ebonylangur", "echinacea", "echiura", "eclectusparrot", "edibledormouse", "eelpout", "egyptiangoose", "eiderduck", "elegantcrested", "elephantshrew", "elkcalf", "ellipsen", "elmborer", "emberiza", "emeraldtree", "emperortamarin", "emydidae", "enchytraeus", "endemicfinch", "entelodont", "eopsaltria", "epeira", "ermineshrew", "erythrura", "estuarinecrocodile"},
	'f': {"fox", "finch", "ferret", "falcon", "frog", "fairywren", "fantail", "fennec", "fieldfare", "firefly", "fisher", "flamingo", "flatfish", "flea", "flicker", "flounder", "fluke", "flycatcher", "flyingfish", "foal", "fossa", "fowl", "francolin", "frigatebird", "frillneck", "fritillary", "frogfish", "frogmouth", "fulmar", "fiddlercrab", "finwhale", "firefish", "fishingcat", "flathead", "fairybluebird", "falconet", "fallowdeer", "fantailfish", "featherstar", "felidae", "fernbird", "fiddlerray", "fieldmouse", "filefish", "finbackwhale", "finchbill", "firebrat", "firecrest", "firefinch", "firesalamander", "fishcrow", "flagtail", "flamefish", "flapjackoctopus", "flatworm", "fleabeetle", "flickertail", "floridapanther", "flowerpecker", "flyriver", "foamnest", "foliagegleaner", "forktail", "fourhorned", "foxsparrow", "foxterrier", "francolinbird", "fregata", "frigate", "frillshark", "fruitbat", "fulvous", "funnelweb", "furseal"},
	'g': {"goat", "goose", "gecko", "gopher", "gull", "gadwall", "galago", "gallinule", "gannet", "gar", "garganey", "gaur", "gavial", "gazelle", "gelada", "gemsbok", "genet", "gerbil", "gerenuk", "gharial", "gibbon", "gila", "giraffe", "glider", "gnat", "gnatcatcher", "gnu", "godwit", "goldcrest", "goldeneye", "goldfinch", "goldfish", "goosander", "goral", "gorilla", "goshawk", "grackle", "grasshopper", "grebe", "greenfinch", "grison", "grosbeak", "grouper", "grouse", "guanaco", "gadfly", "gagfish", "galah", "galapagos", "gallinago", "gambusia", "gangesdolphin", "gardensnake", "garfish", "garterbird", "gastropod", "gazellehound", "geoduck", "gerygone", "ghostbat", "giantpanda", "gilamonster", "ginkgo", "giraffeweevil", "glassfrog", "glowworm", "gnatwren", "goatfish", "goatsucker", "goldentoad", "gonolek", "goosefish", "gorgonian", "goshawkmoth"},
	'h': {"hare", "heron", "hawk", "hedgehog", "husky", "haddock", "hake", "halibut", "hamerkop", "hamster", "hanuman", "harrier", "hartebeest", "hawfinch", "herring", "hoatzin", "hobby", "hog", "hogfish", "hoopoe", "hornbill", "hornet", "horse", "hoverfly", "howler", "huchen", "huemul", "huia", "humpback", "hummingbird", "hutia", "hyacinth", "hyena", "hyrax", "hagfish", "halcyon", "hamadryas", "hammerhead", "harpy", "hellbender", "hackberry", "haddockcod", "hairstreak", "halfbeak", "hammerkopf", "hamsterfish", "hangingparrot", "hardhead", "harlequin", "harpseal", "harvestmouse", "hatchetfish", "hawkmoth", "hawksbill", "hazeldormouse", "heathhen", "hedgesparrow", "helmetshrike", "hemipode", "henharrier", "hercules", "hermitcrab", "herringgull", "hickorytussock", "highlandcow", "hillmyna", "hindwing", "hippopotamus", "hoary", "hogbadger", "honeybadger", "honeyeater", "honeyguide", "hookbill", "hopliadae", "hornedlark", "horseshoebat", "houndfish", "housemartin", "hoverbird", "hummingmoth", "hyaenodon"},
	'i': {"ibis", "iguana", "impala", "ibex", "icefish", "ichneumon", "ide", "iiwi", "inchworm", "indri", "inca", "indigobird", "irukandji", "isopod", "ivorygull", "iguanodon", "ibisbill", "icterine", "impundulu", "incirrata", "iberianlynx", "ibisstork", "icebird", "icelandgull", "ichthyosaur", "ictalurus", "iguanamarine", "impeyan", "indianrhino", "indigobunting", "inland", "insectivore", "ioracommon", "irishwolfhound", "ironwoodbeetle", "isabelline", "islandfox", "ivorybill", "ixodes"},
	'j': {"jay", "jackal", "jaguar", "jabiru", "jacamar", "jacana", "jackdaw", "jackrabbit", "jaeger", "jaguarundi", "janthina", "javelina", "jellyfish", "jerboa", "jewelfish", "jird", "junco", "jungfowl", "juvenile", "jacksnipe", "jacksmelt", "jacksonchameleon", "jaguarcat", "javafinch", "javanrhino", "jawfish", "jerdondove", "jetant", "jewelbeetle", "johndory", "jollytail", "juliabutterfly", "jumpingmouse", "junglecat", "junglecrow", "juniperhairstreak"},
	'k': {"koala", "kiwi", "kestrel", "kudu", "kagu", "kakapo", "kaluga", "kangaroo", "katydid", "kea", "kelpie", "killdeer", "killifish", "kingbird", "kingfisher", "kinglet", "kinkajou", "kiskadee", "kite", "kittiwake", "klipspringer", "knot", "kob", "kodkod", "kookaburra", "kouprey", "krait", "krill", "kulan", "kumquat", "kusimanse", "kakariki", "kalij", "kamikaze", "kangaroorat", "karakul", "katipo", "keelback", "kelpgull", "kentishplover", "kestrelfalcon", "keyhole", "kidfox", "killerwhale", "kingcobra", "kingcrab", "kingeider", "kingpenguin", "kingsnake", "kissingbug", "kittenmoth", "kleptoparasite", "knifefish", "koalabear", "kodiak", "kokanee", "kolibri", "komodo", "kori", "kowari"},
	'l': {"lynx", "lark", "llama", "lemur", "loon", "lacewing", "ladybird", "lamprey", "langur", "lanner", "lapwing", "leafhopper", "lechwe", "leech", "lemming", "leopard", "lerot", "limpet", "ling", "linnet", "lion", "lionfish", "lizard", "loach", "lobster", "locust", "loggerhead", "longspur", "lorikeet", "loris", "lovebird", "lugworm", "lungfish", "lyrebird", "labrador", "lacebug", "lacewinged", "ladyfish", "lakechub", "lammergeier", "lampreyeel", "lancelet", "lanceolated", "langurmonkey", "lapwinged", "larkbunting", "larvacean", "laughingdove", "lavagull", "leafbird", "leaffish", "leatherback", "leatherjacket", "lechweantelope", "leopardgecko", "leopardseal", "lesserjacana", "lethrinops", "lightningbug", "limpkin", "lionhead", "littlebustard", "lizardfish", "lobefin", "longfin", "longhorn", "loriskeet"},
	'm': {"mole", "moose", "mink", "magpie", "marten", "macaque", "macaw", "mackerel", "maholi", "malkoha", "mallard", "mamba", "mammoth", "manakin", "manatee", "mandrill", "mangabey", "manta", "mara", "margay", "marlin", "marmoset", "marmot", "martin", "mayfly", "meadowlark", "meerkat", "megapode", "merganser", "merlin", "midge", "millipede", "minnow", "moccasin", "mockingbird", "mollusk", "mongoose", "monitor", "monkey", "moorhen", "moth", "mouflon", "mouse", "mudskipper", "mule", "mullet", "muntjac", "murre", "muskox", "muskrat", "mussel", "macropod", "madagascan", "magellanic", "magpielark", "mahseer", "maidenhair", "makoshark", "malachite", "mallee", "mammalbat", "manedwolf", "mangrove", "mantidfly", "mantisshrimp", "marbledcat", "marineiguana", "marshharrier", "marsupialmole"},
	'n': {"newt", "narwhal", "nightjar", "nutria", "nabarlek", "nag", "nagapies", "nailtail", "nandu", "natterjack", "nautilus", "needlefish", "nene", "neon", "nerka", "nighthawk", "nightingale", "nilgai", "noctule", "noddy", "numbat", "nutcracker", "nuthatch", "nyala", "nakedmolerat", "nannygoat", "narwhalwhale", "natricine", "naturalist", "nauplius", "nectarbird", "needletail", "nematode", "nesomys", "netdevil", "newtsalamander", "nicator", "nightheron", "nightmonkey", "nilecrocodile", "ninebanded", "noctuid", "noisyminer", "norwayrat", "nosehorn", "notornis", "nubiangoat", "nudibranch", "numbfish", "nursefish", "nutmegmannikin", "nuttallwoodpecker", "nyalaantelope"},
	'o': {"otter", "owl", "oryx", "ocelot", "osprey", "oarfish", "okapi", "olingo", "onager", "opah", "opossum", "orangutan", "orca", "oriole", "oscar", "ostrich", "ouzel", "ovenbird", "ox", "oxpecker", "oystercatcher", "octopus", "oilbird", "olm", "oribi", "oakworm", "oarweed", "obsidianbutterfly", "oceanperch", "ocellated", "octopod", "odonate", "oilfish", "okarito", "oldsquaw", "olivebackedsunbird", "onychophora", "oophaga", "openbill", "orangeroughy", "orbweaver", "orcawhale", "orchidmantis", "oreo", "ornatetinamou", "oropendola", "oryctolagus", "osmia", "ostraciidae", "otterhound", "ouananiche", "ourebi", "ovenbirdnest", "owlmonkey"},
	'p': {"panda", "puma", "pigeon", "pelican", "python", "paca", "pacarana", "paddlefish", "panther", "parakeet", "pardalote", "parrot", "parrotfish", "partridge", "peacock", "peafowl", "peccary", "penguin", "perch", "peregrine", "petrel", "pewee", "phalarope", "pheasant", "pig", "pika", "pike", "pilchard", "pinniped", "pintail", "pipefish", "pipit", "piranha", "pitta", "platypus", "plover", "pochard", "pollock", "pompano", "poorwill", "porcupine", "porpoise", "possum", "potoo", "prairiedog", "pratincole", "prawn", "ptarmigan", "puffin", "pacifichake", "paddycrab", "paintedlady", "palila", "palmcivet", "pampas", "panamaniangolden", "pangolin", "paperwasp", "paradisefish", "parasitoid", "parrotcrossbill", "passerine", "patagonian", "peachfaced", "pearlfish", "pelagicgull", "penguinking", "peppermoth", "perentie", "petaurus", "phasmid", "pheasantcoucal", "phoebe", "pichiciego", "piedwagtail", "pigmyhog", "pikeperch", "pilotwhale", "pinemarten"},
	'q': {"quail", "quokka", "quoll", "quetzal", "quagga", "quillback", "queensnake", "quelea", "quillfish", "quahog", "quillpig", "quinnat", "quailfinch", "quakerparrot"},
	'r': {"raven", "rabbit", "robin", "raccoon", "ragfish", "rail", "rainbowfish", "ram", "rasbora", "rat", "ratel", "rattlesnake", "ray", "razorbill", "redpoll", "redshank", "redstart", "redwing", "reedbuck", "reindeer", "remora", "rhea", "rhinoceros", "roach", "roadrunner", "roan", "rockfish", "roe", "roller", "rook", "rorqual", "rosella", "rotifer", "roughy", "ruff", "ruffe", "rabbitfish", "raccoondog", "ragworm", "rainbowlorikeet", "rattail", "razorclam", "redgrouse", "redkangaroo", "redkite", "redsquirrel", "reeffish", "reefheron", "reticulated", "rhinobeetle", "ribbonseal", "ridgeback", "riflebird", "ringtail", "riverdolphin", "robberfly", "rockdove", "rockhopper", "rosefinch", "roughlegged", "royalpenguin", "rubythroat", "ruddyduck", "ruffedgrouse", "rustyblackbird"},
	's': {"seal", "swan", "stoat", "sparrow", "skunk", "sable", "saiga", "sailfish", "salamander", "salmon", "sandpiper", "sanderling", "sandgrouse", "sapsucker", "sardine", "sawfish", "scallop", "scaup", "scorpion", "scoter", "screamer", "seahorse", "sealion", "serval", "shad", "shark", "shearwater", "sheep", "shelduck", "shrew", "shrike", "shrimp", "siamang", "sifaka", "siskin", "skate", "skimmer", "skink", "skua", "sloth", "slug", "smelt", "snail", "snake", "snipe", "snook", "sole", "spoonbill", "springbok", "squid", "squirrel", "starling", "stingray", "stork", "sturgeon", "sunbird", "swallow", "swift", "swordfish", "sablefish", "sacredibis", "saddleback", "sagegrouse", "saltmarsh", "sandcat", "sanddollar", "sandhillcrane", "sandmartin", "sandtiger", "sapphire", "sawshark", "scarabbeetle", "scarletibis", "scimitar", "scissortail", "scorpionfish", "screechowl", "seabass", "seacow", "seadragon", "seaslug", "seaurchin", "sedgewarbler", "seedeater", "seriema"},
	't': {"toad", "tiger", "tern", "tapir", "tortoise", "tahr", "takin", "tamandua", "tamarin", "tanager", "tarantula", "tarpon", "tarsier", "teal", "tench", "tenrec", "termite", "terrapin", "tetra", "thrasher", "thrush", "tilapia", "tinamou", "titmouse", "toadfish", "tody", "topi", "toucan", "towhee", "tragopan", "treecreeper", "trogon", "trout", "tuatara", "tuna", "turaco", "turkey", "turnstone", "turtle", "tusk", "tussock", "tabbycat", "tadpoleshrimp", "tailorbird", "takahe", "talapoin", "tamarack", "tanuki", "tapaculo", "tarantulahawk", "tasmaniandevil", "tawnyowl", "tegulizard", "telescopefish", "temminck", "tessellated", "thickknee", "thornbill", "thornytoad", "thresher", "tigerbeetle", "tigermoth", "tigershark", "timberwolf", "titi", "toadbug", "tomtit", "tonguefish", "topminnow", "torrentduck", "tortoiseshell"},
	'u': {"urchin", "urial", "uakari", "umbrellabird", "unau", "uguisu", "urutu", "ural", "uromastyx", "urubu", "ukari", "ular", "umbrette", "unicornfish", "upupa", "urocyon", "ursine"},
	'v': {"vole", "viper", "vulture", "vixen", "vampirebat", "vanga", "vaquita", "veery", "velvetworm", "verdin", "vervet", "vicuna", "vinegaroon", "vireo", "viscacha", "volvox", "vulpes", "verditer"},
	'w': {"wren", "wolf", "walrus", "weasel", "wombat", "wagtail", "wallaby", "wallaroo", "warbler", "warthog", "waterbuck", "waterbug", "wattlebird", "waxbill", "waxwing", "weevil", "whale", "whimbrel", "whinchat", "whippet", "whipsnake", "whiteeye", "whiting", "widgeon", "wildebeest", "willet", "wolverine", "woodchuck", "woodcock", "woodlouse", "woodpecker", "worm", "wrasse", "wryneck", "wagtailbird", "wanderingalbatross", "warblerfinch", "wartyfrog", "waterboatman", "waterdragon", "watermoccasin", "waterstrider", "wattledcrane", "waxmoth", "weaverbird", "webspinner", "wedgetailed", "weddellseal", "wellsfargo", "westerngorilla", "whalefish", "wheatear", "whiptail", "whiskeredtern", "whitebait", "whiteshark", "whooper", "widowbird", "wigeon", "wildcat", "willowtit", "windowpane", "wingedsnail", "winterwren", "wireworm", "wisent", "wolffish", "woodswallow", "woollymammoth", "wormlizard"},
	'x': {"xerus", "xenops", "xantus", "xoloitzcuintli", "xiphias", "xenopus", "xema", "xantusia", "xylocopa", "xiphosura", "xerocoris", "xenarthra", "xanthareel", "xenurine"},
	'y': {"yak", "yellowjacket", "yabby", "yaffle", "yapok", "yellowhammer", "yellowlegs", "yellowtail", "yellowthroat", "yeti", "yakuza", "yearling", "yuhina", "yabbie", "yacare", "yaguarundi"},
	'z': {"zebra", "zorilla", "zebu", "zander", "zeren", "zebrafinch", "zebrafish", "zigzagheron", "zokor", "zonure", "zooplankton", "zorro", "zebradove"},
}

var alphabet = func() []byte {
	letters := make([]byte, 0, 26)
	for c := byte('a'); c <= 'z'; c++ {
		letters = append(letters, c)
	}
	return letters
}()

// nameState is the persisted record: every codename handed out so far
// (shared, machine-wide — this is what "globally unique" means here), each
// project's own next-letter position in ITS alphabetical sequence (keyed
// by absolute project directory), and whether the one-time import of
// pre-existing on-disk plan names has run yet.
type nameState struct {
	Used           map[string]bool `json:"used"`
	ProjectNextIdx map[string]int  `json:"project_next_idx"`
	Seeded         bool            `json:"seeded"`
}

// NamesPath is the global registry file every NextPlanName call reads and
// writes — under StoreDir(), same as events.jsonl/labels.json, so it's one
// shared sequence machine-wide rather than per-project.
func NamesPath() string {
	return filepath.Join(StoreDir(), "plan-names.json")
}

func namesLockPath() string {
	return NamesPath() + ".lock"
}

// NextPlanName claims and returns the next codename in projectDir's OWN
// alphabetical sequence (a, b, c, ... z, a, b, ... — that project's 1st new
// plan is an a-word, 2nd a b-word, and so on), never reissuing a word
// already assigned to ANY project on the machine. The very first call
// (from any project) seeds the "already used" set from every plan name
// already on disk across every known project, so names in use before this
// registry existed are never handed out again.
func NextPlanName(projectDir string) (string, error) {
	return nextPlanName(NamesPath(), namesLockPath(), projectDir, seedFromKnownProjects)
}

// seedFromKnownProjects collects every plan name already on disk across
// every project iterate-run knows about — best-effort, same tolerance for
// a missing/unreadable project as the rest of this package.
func seedFromKnownProjects() map[string]bool {
	used := map[string]bool{}
	projects, err := ListProjects()
	if err != nil {
		return used
	}
	for _, proj := range projects {
		plans, err := ListPlans(proj)
		if err != nil {
			continue
		}
		for _, p := range plans {
			if p.Name != "" {
				used[p.Name] = true
			}
		}
	}
	return used
}

// nextPlanName is NextPlanName's testable core — path, lockPath, and the
// seed function are injected so tests exercise the real algorithm
// (locking, seeding, per-project alphabetical cycling, persistence)
// against a throwaway directory instead of the machine's real global
// registry.
func nextPlanName(path, lockPath, projectDir string, seed func() map[string]bool) (string, error) {
	unlock, err := lockFile(lockPath)
	if err != nil {
		return "", err
	}
	defer unlock()

	st, err := loadNameState(path)
	if err != nil {
		return "", err
	}
	if !st.Seeded {
		for name := range seed() {
			st.Used[name] = true
		}
		st.Seeded = true
	}

	key := projectKey(projectDir)
	startIdx := st.ProjectNextIdx[key]

	for i := range len(alphabet) {
		idx := (startIdx + i) % len(alphabet)
		for _, name := range animalsByLetter[alphabet[idx]] {
			if st.Used[name] {
				continue
			}
			if i > 0 {
				// The letter this project's sequence would naturally land
				// on next (alphabet[startIdx]) has no unused names left
				// machine-wide. Rather than fail the caller, advance to
				// the next letter that still has capacity — but say so,
				// since a silent skip is indistinguishable from a bug.
				fmt.Fprintf(os.Stderr, "iterate-run: letter %q exhausted machine-wide, advancing to %q\n", alphabet[startIdx], alphabet[idx])
			}
			st.Used[name] = true
			st.ProjectNextIdx[key] = (idx + 1) % len(alphabet)
			if err := saveNameState(path, st); err != nil {
				return "", err
			}
			return name, nil
		}
	}
	return "", fmt.Errorf("iterate-run: every animal codename in the pool is already in use (extend animalsByLetter in names.go) — remaining per letter: %s", formatRemainingByLetter(remainingByLetter(st.Used)))
}

// remainingByLetter reports, for every letter in the alphabet, how many
// names in that letter's pool are not yet in used. Only meaningful to call
// once the whole-alphabet scan in nextPlanName has already failed to find
// any name — at that point every count it reports is 0, and the point is
// to make that visible per letter rather than as one opaque failure, so
// the next pool extension is sized from real data instead of a guess.
func remainingByLetter(used map[string]bool) map[byte]int {
	counts := make(map[byte]int, len(alphabet))
	for _, letter := range alphabet {
		n := 0
		for _, name := range animalsByLetter[letter] {
			if !used[name] {
				n++
			}
		}
		counts[letter] = n
	}
	return counts
}

// formatRemainingByLetter renders remainingByLetter's counts as one
// compact "a=0 b=0 c=3 ..." line for the pool-exhausted error message.
func formatRemainingByLetter(counts map[byte]int) string {
	var b strings.Builder
	for i, letter := range alphabet {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%c=%d", letter, counts[letter])
	}
	return b.String()
}

// projectKey normalizes projectDir to an absolute path so the same project
// always maps to the same entry in ProjectNextIdx regardless of which
// relative path or cwd a given call happened to use. Falls back to the raw
// string on a resolution error rather than failing the whole call.
func projectKey(projectDir string) string {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return projectDir
	}
	return abs
}

func loadNameState(path string) (*nameState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &nameState{Used: map[string]bool{}, ProjectNextIdx: map[string]int{}}, nil
		}
		return nil, err
	}
	var st nameState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	if st.Used == nil {
		st.Used = map[string]bool{}
	}
	if st.ProjectNextIdx == nil {
		st.ProjectNextIdx = map[string]int{}
	}
	return &st, nil
}

func saveNameState(path string, st *nameState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// lockFile takes an exclusive advisory lock on path (created if needed) so
// two concurrent `iterate-run name next` calls — from two different
// projects racing to create a plan at the same moment — can't both read
// the same state and hand out the same name. The returned func releases
// it; always call it via defer.
func lockFile(path string) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
